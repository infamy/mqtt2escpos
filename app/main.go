package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gosixtyfive/escpos"
)

// PrintJob represents the structure of MQTT print messages
type PrintJob struct {
	PrinterName string `json:"printer_name"`
	Title       string `json:"title,omitempty"`
	Message     string `json:"message"`
	Format      string `json:"format,omitempty"` // "text", "qr", "barcode", "image"
	Size        int    `json:"size,omitempty"`   // text size (1-8)
	Bold        bool   `json:"bold,omitempty"`
	Underline   bool   `json:"underline,omitempty"`
	Justify     string `json:"justify,omitempty"` // "left", "center", "right"
	QRCode      string `json:"qr_code,omitempty"`
	Barcode     string `json:"barcode,omitempty"`
	Image       string `json:"image,omitempty"`        // base64 encoded image data
	ImageFormat string `json:"image_format,omitempty"` // "png", "jpeg", "gif"
}

// PrinterCommand represents printer control commands
type PrinterCommand struct {
	PrinterName string `json:"printer_name"`
	Command     string `json:"command"`         // "feed", "cut", "feed_and_cut"
	Lines       int    `json:"lines,omitempty"` // Number of lines to feed (for feed command)
}

// PrinterProfile defines printer capabilities and settings
type PrinterProfile struct {
	Name         string
	MaxWidth     int     // Maximum printable width in dots
	MaxHeight    int     // Maximum printable height in dots
	DefaultWidth int     // Default image width to use
	ImageMode    string  // "bitimage", "raster"
	Threshold    int     // Grayscale threshold (0-255)
	UseDithering bool    // Whether to use dithering
	DitherMatrix [][]int // Dithering matrix
	EscposConfig escpos.PrinterConfig
}

// Common printer profiles
var PrinterProfiles = map[string]PrinterProfile{
	"TM-T20II": {
		Name:         "TM-T20II",
		EscposConfig: escpos.ConfigEpsonTMT20II, // supports upside-down printing
		MaxWidth:     384,
		MaxHeight:    1024,
		DefaultWidth: 384,
		ImageMode:    "bitimage",
		Threshold:    128,
		UseDithering: false, // Temporarily disabled to test
		DitherMatrix: [][]int{
			{0, 8, 2, 10},
			{12, 4, 14, 6},
			{3, 11, 1, 9},
			{15, 7, 13, 5},
		},
	},
	"TM-T88III": {
		Name:         "TM-T88III",
		EscposConfig: escpos.ConfigEpsonTMT20II, // supports upside-down printing
		MaxWidth:     512,
		MaxHeight:    1024,
		DefaultWidth: 512,
		ImageMode:    "bitimage",
		Threshold:    128,
		UseDithering: false, // Temporarily disabled to test
		DitherMatrix: [][]int{
			{0, 8, 2, 10},
			{12, 4, 14, 6},
			{3, 11, 1, 9},
			{15, 7, 13, 5},
		},
	},
	"TM-T88VI": {
		Name:         "TM-T88VI",
		EscposConfig: escpos.ConfigEpsonTMT20II, // supports upside-down printing
		MaxWidth:     576,
		MaxHeight:    1024,
		DefaultWidth: 576,
		ImageMode:    "bitimage",
		Threshold:    128,
		UseDithering: false, // Temporarily disabled to test
		DitherMatrix: [][]int{
			{0, 8, 2, 10},
			{12, 4, 14, 6},
			{3, 11, 1, 9},
			{15, 7, 13, 5},
		},
	},
}

// Config holds the application configuration
type Config struct {
	MQTTBroker     string
	MQTTUsername   string
	MQTTPassword   string
	MQTTTopic      string
	PrinterHost    string
	PrinterPort    string
	PrinterProfile string        // Printer profile to use
	ImageTimeout   time.Duration // write deadline for jobs containing an image
	MaxImageSize   int           // maximum size of the base64 image payload, in bytes
	HealthAddr     string        // listen address for the HTTP health endpoint ("" disables it)
	ClientID       string        // fixed MQTT client ID so the broker keeps our session
	HADiscovery    bool          // publish Home Assistant MQTT discovery configs
	StatusInterval time.Duration // how often to poll the printer status
	Version        string
}

// PrinterClient handles the printer connection and printing
type PrinterClient struct {
	mu      sync.Mutex // one printer connection at a time (jobs and status polls)
	config  Config
	conn    net.Conn
	printer *escpos.Escpos
	profile PrinterProfile
}

// NewPrinterClient creates a new printer client
func NewPrinterClient(config Config) *PrinterClient {
	profile, exists := PrinterProfiles[config.PrinterProfile]
	if !exists {
		// Default to TM-T20II if profile not found
		profile = PrinterProfiles["TM-T20II"]
		log.Printf("Warning: Printer profile '%s' not found, using default TM-T20II", config.PrinterProfile)
	}

	return &PrinterClient{
		config:  config,
		profile: profile,
	}
}

// Connect establishes connection to the printer
func (pc *PrinterClient) Connect() error {
	// Close existing connection if any
	if pc.conn != nil {
		pc.conn.Close()
		pc.conn = nil
		pc.printer = nil
	}

	var err error
	pc.conn, err = net.DialTimeout("tcp", pc.addr(), 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to printer: %v", err)
	}

	// Enable TCP keepalive to detect dead connections
	if tcpConn, ok := pc.conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	pc.printer = escpos.New(pc.conn)
	pc.printer.SetConfig(pc.profile.EscposConfig)

	log.Printf("Connected to printer at %s:%s using profile: %s", pc.config.PrinterHost, pc.config.PrinterPort, pc.profile.Name)
	return nil
}

func (pc *PrinterClient) addr() string {
	return net.JoinHostPort(pc.config.PrinterHost, pc.config.PrinterPort)
}

// Close closes the printer connection. Many network printers accept only one
// connection at a time, so we close after every job rather than holding it open.
func (pc *PrinterClient) Close() error {
	if pc.conn == nil {
		return nil
	}
	err := pc.conn.Close()
	pc.conn = nil
	pc.printer = nil
	return err
}

// Feed queues a paper feed of the specified number of lines
func (pc *PrinterClient) Feed(lines int) {
	if lines <= 0 {
		lines = 1 // Default to 1 line if not specified
	}
	log.Printf("Feeding paper by %d lines", lines)
	for i := 0; i < lines; i++ {
		pc.printer.LineFeed()
	}
}

// Cut queues a paper cut (the printer feeds to the cutter position first)
func (pc *PrinterClient) Cut() {
	log.Printf("Cutting paper")
	pc.printer.Cut()
}

// waitUntilProcessed flushes everything buffered for the printer, then sends
// GS r 1 (transmit paper sensor status). Unlike the real-time DLE EOT queries,
// the printer answers GS r only once it has processed all the data in front of
// it, so the reply tells us the job is safely in the printer. Closing the
// connection earlier can make the printer discard what it hasn't printed yet
// (this is what used to lose images). Waits until the connection deadline.
func (pc *PrinterClient) waitUntilProcessed() error {
	start := time.Now()
	pc.printer.WriteRaw([]byte{0x1D, 0x72, 0x01})
	if err := pc.printer.Print(); err != nil {
		return fmt.Errorf("failed to send data to printer: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(pc.conn, buf); err != nil {
		// The data has been sent; we just can't confirm it. Don't fail the job.
		log.Printf("Warning: printer did not confirm the job after %s: %v", time.Since(start).Round(time.Millisecond), err)
		return nil
	}
	log.Printf("Printer confirmed job in %s", time.Since(start).Round(time.Millisecond))
	return nil
}

// ExecuteCommand executes a printer command
func (pc *PrinterClient) ExecuteCommand(cmd PrinterCommand) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Fresh connection for every command
	log.Printf("Connecting to printer for command execution...")
	if err := pc.Connect(); err != nil {
		return err
	}
	defer pc.Close()
	pc.conn.SetDeadline(time.Now().Add(15 * time.Second))

	log.Printf("Executing printer command: %s", cmd.Command)

	switch strings.ToLower(cmd.Command) {
	case "feed":
		pc.Feed(cmd.Lines)
	case "cut":
		pc.Cut()
	case "feed_and_cut":
		pc.Feed(cmd.Lines)
		pc.Cut()
	default:
		return fmt.Errorf("unknown printer command: %s", cmd.Command)
	}
	return pc.waitUntilProcessed()
}

// resizeImage resizes an image to fit within printer limits using proper scaling
func (pc *PrinterClient) resizeImage(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// If image is already within limits, return as is
	if width <= pc.profile.MaxWidth && height <= pc.profile.MaxHeight {
		return img
	}

	// Calculate scaling factor to fit within limits
	scaleX := float64(pc.profile.MaxWidth) / float64(width)
	scaleY := float64(pc.profile.MaxHeight) / float64(height)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	// Calculate new dimensions
	newWidth := int(float64(width) * scale)
	newHeight := int(float64(height) * scale)

	// Ensure width is multiple of 8 for ESC/POS compatibility
	newWidth = (newWidth / 8) * 8
	if newWidth < 8 {
		newWidth = 8
	}

	log.Printf("Resizing image from %dx%d to %dx%d", width, height, newWidth, newHeight)

	// Create a new RGBA image with the target size
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// Proper scaling: map each destination pixel to source pixels
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			// Map destination coordinates to source coordinates
			srcX := int(float64(x) / scale)
			srcY := int(float64(y) / scale)

			// Ensure we don't go out of bounds
			if srcX >= width {
				srcX = width - 1
			}
			if srcY >= height {
				srcY = height - 1
			}

			// Get the source pixel and convert to RGBA
			srcColor := img.At(srcX, srcY)
			r, g, b, a := srcColor.RGBA()

			// Convert to 8-bit values and set in the new image
			resized.Set(x, y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			})
		}
	}

	return resized
}

// applyDithering applies Floyd-Steinberg dithering to the image
func (pc *PrinterClient) applyDithering(img image.Image) image.Image {
	if !pc.profile.UseDithering {
		return img
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Convert to grayscale and create error diffusion matrix
	grayscale := image.NewGray(bounds)
	errors := make([][]float64, height)
	for y := 0; y < height; y++ {
		errors[y] = make([]float64, width)
	}

	// First pass: convert to grayscale
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			gray := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
			grayscale.Set(x, y, color.Gray{Y: uint8(gray)})
		}
	}

	// Second pass: apply Floyd-Steinberg dithering
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			oldPixel := float64(grayscale.GrayAt(x, y).Y) + errors[y][x]
			var newPixel float64
			if oldPixel > float64(pc.profile.Threshold) {
				newPixel = 255
			} else {
				newPixel = 0
			}
			grayscale.Set(x, y, color.Gray{Y: uint8(newPixel)})

			quantError := oldPixel - newPixel

			// Distribute error to neighboring pixels
			if x+1 < width {
				errors[y][x+1] += quantError * 7.0 / 16.0
			}
			if x-1 >= 0 && y+1 < height {
				errors[y+1][x-1] += quantError * 3.0 / 16.0
			}
			if y+1 < height {
				errors[y+1][x] += quantError * 5.0 / 16.0
			}
			if x+1 < width && y+1 < height {
				errors[y+1][x+1] += quantError * 1.0 / 16.0
			}
		}
	}

	return grayscale
}

// decodeImage decodes a base64 encoded image string with improved processing
func (pc *PrinterClient) decodeImage(imageData, format string) (image.Image, error) {
	// Decode base64 data
	data, err := base64.StdEncoding.DecodeString(imageData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 image data: %v", err)
	}

	log.Printf("Decoded image data: %d bytes", len(data))

	// Decode image based on format
	var img image.Image
	switch strings.ToLower(format) {
	case "png":
		img, err = png.Decode(bytes.NewReader(data))
	case "jpeg", "jpg":
		img, err = jpeg.Decode(bytes.NewReader(data))
	case "gif":
		img, err = gif.Decode(bytes.NewReader(data))
	default:
		return nil, fmt.Errorf("unsupported image format: %s", format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	// Process the image
	img = pc.resizeImage(img)
	img = pc.applyDithering(img)

	return img, nil
}

// Print processes a print job with fresh connection for each job
func (pc *PrinterClient) Print(job PrintJob) error {
	if len(job.Image) > pc.config.MaxImageSize {
		return fmt.Errorf("image is %d bytes, larger than max_image_size (%d bytes); job rejected",
			len(job.Image), pc.config.MaxImageSize)
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Fresh connection for every job
	log.Printf("Connecting to printer for print job...")
	if err := pc.Connect(); err != nil {
		return err
	}
	defer pc.Close()

	timeout := 15 * time.Second
	if job.Image != "" {
		timeout = pc.config.ImageTimeout
	}
	pc.conn.SetDeadline(time.Now().Add(timeout))

	// Print the job
	err := pc.printJobInternal(job)
	if err != nil {
		log.Printf("Print job failed with error: %v", err)
		return err
	}

	return nil
}

// printJobInternal contains the original print logic
func (pc *PrinterClient) printJobInternal(job PrintJob) error {
	if pc.printer == nil {
		return fmt.Errorf("printer not connected")
	}

	log.Printf("Printing job for printer: %s", job.PrinterName)
	if job.Title != "" {
		log.Printf("Title: %s", job.Title)
	}
	log.Printf("Message: %s", truncate(job.Message, 200))

	// Configure text formatting
	if job.Bold {
		pc.printer.Bold(true)
	}

	if job.Underline {
		pc.printer.Underline(2)
	}

	// Set text size
	if job.Size > 0 && job.Size <= 8 {
		pc.printer.Size(uint8(job.Size), uint8(job.Size))
	}

	// Print title if specified (centered and bold)
	if job.Title != "" {
		// Save current formatting
		pc.printer.Justify(escpos.JustifyCenter)
		pc.printer.Bold(true)
		pc.printer.Size(2, 2)

		pc.printer.Write(job.Title)
		pc.printer.LineFeed()
		pc.printer.LineFeed()

		// Reset formatting
		pc.printer.Bold(false)
		pc.printer.Size(1, 1)
	}

	// Print image if specified (at the top, after title)
	if job.Image != "" {
		log.Printf("Processing image for printing")

		// Determine image format
		format := job.ImageFormat
		if format == "" {
			// Try to detect format from base64 data or default to PNG
			format = "png"
		}

		// Decode the image
		img, err := pc.decodeImage(job.Image, format)
		if err != nil {
			log.Printf("Error decoding image: %v", err)
			pc.printer.Write("Error: Failed to decode image")
			pc.printer.LineFeed()
		} else {
			// Print the image
			_, err = pc.printer.PrintImage(img)
			if err != nil {
				log.Printf("Error printing image: %v", err)
				pc.printer.Write("Error: Failed to print image")
				pc.printer.LineFeed()
			} else {
				log.Printf("Image queued for printing")
				pc.printer.LineFeed()
			}
		}
	}

	// Set justification for message
	switch job.Justify {
	case "center":
		pc.printer.Justify(escpos.JustifyCenter)
	case "right":
		pc.printer.Justify(escpos.JustifyRight)
	default:
		pc.printer.Justify(escpos.JustifyLeft)
	}

	// Print the message
	if job.Message != "" {
		// Split message by newlines and print each line
		lines := strings.Split(job.Message, "\n")
		for i, line := range lines {
			if line != "" {
				pc.printer.Write(line)
			}
			// Add line feed for all lines except the last one
			if i < len(lines)-1 {
				pc.printer.LineFeed()
			}
		}
		// Add final line feed after the message
		pc.printer.LineFeed()
	}

	// Print QR code if specified
	if job.QRCode != "" {
		pc.printer.QRCode(job.QRCode, true, 10, escpos.QRCodeErrorCorrectionLevelH)
		pc.printer.LineFeed()
	}

	// Print barcode if specified (using EAN13 as default)
	if job.Barcode != "" {
		// Try to print as EAN13 barcode (most common)
		if len(job.Barcode) == 13 {
			pc.printer.EAN13(job.Barcode)
		} else if len(job.Barcode) == 8 {
			pc.printer.EAN8(job.Barcode)
		} else if len(job.Barcode) == 12 {
			pc.printer.UPCA(job.Barcode)
		} else {
			// For other formats, just print as text
			pc.printer.Write("Barcode: " + job.Barcode)
		}
		pc.printer.LineFeed()
	}

	// Reset formatting
	pc.printer.Bold(false)
	pc.printer.Underline(0)
	pc.printer.Justify(escpos.JustifyLeft)
	pc.printer.Size(1, 1)

	// Send to printer (without cutting) and wait until it has taken everything
	if err := pc.waitUntilProcessed(); err != nil {
		log.Printf("Error during print: %v", err)
		return err
	}

	log.Printf("Print job completed successfully")
	return nil
}

// MQTTClient handles MQTT communication
type MQTTClient struct {
	client     mqtt.Client
	config     Config
	printer    *PrinterClient
	subscribed atomic.Bool
	refresh    chan struct{} // asks the status poller to check the printer now
}

// NewMQTTClient creates a new MQTT client
func NewMQTTClient(config Config, printer *PrinterClient) *MQTTClient {
	return &MQTTClient{
		config:  config,
		printer: printer,
		refresh: make(chan struct{}, 1),
	}
}

// brokerURL normalises the configured broker address, adding the default
// MQTT port when none is given (e.g. "core-mosquitto" -> "tcp://core-mosquitto:1883").
func brokerURL(broker string) string {
	if !strings.Contains(broker, "://") {
		broker = "tcp://" + broker
	}
	scheme, host, _ := strings.Cut(broker, "://")
	if _, _, err := net.SplitHostPort(host); err != nil {
		port := "1883"
		if scheme == "ssl" || scheme == "tls" || scheme == "mqtts" {
			port = "8883"
		}
		host = net.JoinHostPort(host, port)
	}
	return scheme + "://" + host
}

// Connect starts the MQTT connection. It does not block: the client keeps
// retrying in the background until the broker is reachable, and (re)subscribes
// every time a connection is established. This lets the bridge start before the
// broker is up (e.g. on a fresh Home Assistant boot) and survive broker restarts.
func (mc *MQTTClient) Connect() {
	broker := brokerURL(mc.config.MQTTBroker)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetUsername(mc.config.MQTTUsername)
	opts.SetPassword(mc.config.MQTTPassword)
	// A fixed client ID plus a persistent session (CleanSession=false) means the
	// broker queues QoS 1 print jobs sent while we're restarting or reconnecting.
	opts.SetClientID(mc.config.ClientID)
	opts.SetCleanSession(false)
	// Handle messages even if they arrive before Subscribe completes: queued
	// messages are delivered right after connecting. We only subscribe to the
	// print topic, so everything that arrives here is a print job or command.
	opts.SetDefaultPublishHandler(mc.messageHandler)
	// Broker marks us offline if we disappear without a clean disconnect.
	opts.SetWill(topicBridgeStatus, "offline", 1, true)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetConnectTimeout(10 * time.Second)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Printf("Connected to MQTT broker at %s", broker)
		go func() {
			mc.publish(topicBridgeStatus, "online", true)
			if mc.config.HADiscovery {
				mc.publishDiscovery()
			}
			// Subscribe on every (re)connect in case the broker lost our session.
			if err := mc.Subscribe(); err != nil {
				log.Printf("Error: %v", err)
			}
			mc.requestStatusRefresh()
		}()
	})
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		mc.subscribed.Store(false)
		log.Printf("Lost connection to MQTT broker: %v (will reconnect)", err)
	})
	opts.SetReconnectingHandler(func(c mqtt.Client, o *mqtt.ClientOptions) {
		log.Printf("Reconnecting to MQTT broker at %s...", broker)
	})
	opts.SetConnectionAttemptHandler(func(b *url.URL, cfg *tls.Config) *tls.Config {
		log.Printf("Attempting MQTT connection to %s", b.String())
		return cfg
	})

	mc.client = mqtt.NewClient(opts)
	mc.client.Connect() // non-blocking with ConnectRetry; OnConnect handles the rest
}

// Subscribe subscribes to the print topic
func (mc *MQTTClient) Subscribe() error {
	token := mc.client.Subscribe(mc.config.MQTTTopic, 1, mc.messageHandler)
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("timed out subscribing to topic %s", mc.config.MQTTTopic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %v", mc.config.MQTTTopic, err)
	}

	mc.subscribed.Store(true)
	log.Printf("Subscribed to MQTT topic: %s", mc.config.MQTTTopic)
	return nil
}

// Healthy reports whether the bridge is connected to the broker and subscribed.
func (mc *MQTTClient) Healthy() bool {
	return mc.client != nil && mc.client.IsConnectionOpen() && mc.subscribed.Load()
}

// startHealthServer serves GET /health: 200 when MQTT is connected and
// subscribed, 503 otherwise. Used by the Supervisor watchdog and Docker HEALTHCHECK.
func startHealthServer(addr string, mc *MQTTClient) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]any{
			"mqtt_connected":  mc.client != nil && mc.client.IsConnectionOpen(),
			"mqtt_subscribed": mc.subscribed.Load(),
		}
		w.Header().Set("Content-Type", "application/json")
		if mc.Healthy() {
			status["status"] = "ok"
			w.WriteHeader(http.StatusOK)
		} else {
			status["status"] = "unhealthy"
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(status)
	})

	go func() {
		log.Printf("Health endpoint listening on %s/health", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("Health endpoint stopped: %v", err)
		}
	}()
}

// messageHandler processes incoming MQTT messages
func (mc *MQTTClient) messageHandler(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received %d-byte message on topic %s", len(msg.Payload()), msg.Topic())
	defer mc.requestStatusRefresh()

	// Try to parse as a printer command first
	var cmd PrinterCommand
	if err := json.Unmarshal(msg.Payload(), &cmd); err == nil && cmd.Command != "" {
		// This is a printer command
		if cmd.PrinterName == "" {
			log.Printf("Warning: printer_name not specified in command")
		}

		if err := mc.printer.ExecuteCommand(cmd); err != nil {
			log.Printf("Error executing printer command: %v", err)
		}
		return
	}

	// Try to parse as a print job
	var job PrintJob
	if err := json.Unmarshal(msg.Payload(), &job); err != nil {
		log.Printf("Error parsing JSON message: %v (payload: %s)", err, truncate(string(msg.Payload()), 200))
		return
	}

	if job.PrinterName == "" {
		log.Printf("Warning: printer_name not specified in message")
	}

	if job.Message == "" && job.QRCode == "" && job.Barcode == "" && job.Image == "" {
		log.Printf("Warning: no content to print (message, qr_code, barcode, or image)")
		return
	}

	if err := mc.printer.Print(job); err != nil {
		log.Printf("Error printing job: %v", err)
	}
}

// Disconnect disconnects from MQTT broker
func (mc *MQTTClient) Disconnect() {
	if mc.client != nil {
		if mc.client.IsConnectionOpen() {
			// A clean disconnect doesn't trigger the will, so mark ourselves offline.
			mc.publish(topicBridgeStatus, "offline", true)
		}
		mc.client.Disconnect(250)
	}
}

// getConfig loads configuration from environment variables
func getConfig() Config {
	hostname, _ := os.Hostname()
	return Config{
		MQTTBroker:     getEnv("MQTT_BROKER", "localhost:1883"),
		MQTTUsername:   getEnv("MQTT_USERNAME", ""),
		MQTTPassword:   getEnv("MQTT_PASSWORD", ""),
		MQTTTopic:      getEnv("MQTT_TOPIC", "printer/commands"),
		PrinterHost:    getEnv("PRINTER_HOST", "192.168.7.25"),
		PrinterPort:    getEnv("PRINTER_PORT", "9100"),
		PrinterProfile: getEnv("PRINTER_PROFILE", "TM-T88VI"),
		ImageTimeout:   time.Duration(getEnvInt("IMAGE_TIMEOUT", 30)) * time.Second,
		MaxImageSize:   getEnvInt("MAX_IMAGE_SIZE", 1024*1024),
		HealthAddr:     getEnv("HEALTH_ADDR", ":8099"),
		ClientID:       getEnv("MQTT_CLIENT_ID", "mqtt2escpos-"+hostname),
		HADiscovery:    getEnv("HA_DISCOVERY", "true") == "true",
		StatusInterval: time.Duration(getEnvInt("STATUS_INTERVAL", 60)) * time.Second,
		Version:        getEnv("ADDON_VERSION", "dev"),
	}
}

// getEnvInt gets a positive integer environment variable with fallback
func getEnvInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}

// truncate shortens s for logging
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("... (%d bytes)", len(s))
}

// getEnv gets environment variable with fallback
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	log.Println("Starting MQTT to ESC/POS printer bridge...")

	config := getConfig()
	log.Printf("Configuration:")
	log.Printf("  MQTT Broker: %s", config.MQTTBroker)
	log.Printf("  MQTT Topic: %s", config.MQTTTopic)
	log.Printf("  Printer: %s:%s", config.PrinterHost, config.PrinterPort)
	log.Printf("  Printer Profile: %s", config.PrinterProfile)
	log.Printf("  Max Image Size: %d bytes", config.MaxImageSize)
	log.Printf("  Image Timeout: %s", config.ImageTimeout)
	log.Printf("  MQTT Client ID: %s", config.ClientID)
	log.Printf("  HA Discovery: %t (status every %s)", config.HADiscovery, config.StatusInterval)

	// Create printer client. We don't connect here: every job opens a fresh
	// connection, and failing hard at startup (printer asleep, network not up
	// yet on boot) would just crash the add-on.
	printer := NewPrinterClient(config)
	defer printer.Close()

	// Connect to MQTT broker (retries in the background until it's available)
	mqttClient := NewMQTTClient(config, printer)
	if config.HealthAddr != "" {
		startHealthServer(config.HealthAddr, mqttClient)
	}
	mqttClient.Connect()
	defer mqttClient.Disconnect()
	go mqttClient.runStatusPoller()

	log.Println("MQTT to ESC/POS bridge is running. Press Ctrl+C to exit.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
}
