package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"errors"

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
}

// Common printer profiles
var PrinterProfiles = map[string]PrinterProfile{
	"TM-T20II": {
		Name:         "TM-T20II",
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
	PrinterProfile string // Printer profile to use
	ImageTimeout   time.Duration
	MaxImageSize   int
}

// PrinterClient handles the printer connection and printing
type PrinterClient struct {
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
	pc.conn, err = net.Dial("tcp", pc.config.PrinterHost+":"+pc.config.PrinterPort)
	if err != nil {
		return fmt.Errorf("failed to connect to printer: %v", err)
	}

	// Enable TCP keepalive to detect dead connections
	if tcpConn, ok := pc.conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		tcpConn.SetLinger(0) // Don't wait on close
	}

	pc.printer = escpos.New(pc.conn)
	pc.printer.SetConfig(escpos.ConfigEpsonTMT20II)

	log.Printf("Connected to printer at %s:%s using profile: %s", pc.config.PrinterHost, pc.config.PrinterPort, pc.profile.Name)
	return nil
}

// Close closes the printer connection
func (pc *PrinterClient) Close() error {
	if pc.conn != nil {
		return pc.conn.Close()
	}
	return nil
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
	// Check image data size
	dataSize := len(imageData)
	if dataSize > pc.config.MaxImageSize {
		log.Printf("Warning: Image data size (%d bytes) exceeds maximum (%d bytes)", dataSize, pc.config.MaxImageSize)
	}

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

// isConnectionError checks if an error is likely due to a lost printer connection using error types only
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Unwrap and check for net.OpError
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		// Timeout or temporary errors are likely connection issues
		if netErr.Timeout() || netErr.Temporary() {
			return true
		}
		// Check for syscall errors
		var sysErr syscall.Errno
		if errors.As(netErr.Err, &sysErr) {
			switch sysErr {
			case syscall.EPIPE, syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.ENOTCONN, syscall.ETIMEDOUT:
				return true
			}
		}
	}
	// Optionally, check for direct syscall errors
	if errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ENOTCONN) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}
	return false
}

// checkConnectionHealth proactively checks if the printer connection is still alive
func (pc *PrinterClient) checkConnectionHealth() error {
	if pc.conn == nil {
		return fmt.Errorf("printer not connected")
	}

	// Try to get the connection state
	if pc.conn.RemoteAddr() == nil {
		return fmt.Errorf("printer connection lost")
	}

	// Set a short timeout for the health check
	pc.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	defer pc.conn.SetWriteDeadline(time.Time{}) // Clear the deadline

	// Try to send a harmless ESC/POS command (line feed) to test the connection
	_, err := pc.conn.Write([]byte{0x0A}) // Line feed character
	if err != nil {
		return fmt.Errorf("connection health check failed: %v", err)
	}

	return nil
}

// Print processes a print job with fresh connection for each job
func (pc *PrinterClient) Print(job PrintJob) error {
	// Reconnect before every print job to ensure fresh connection
	log.Printf("Connecting to printer for print job...")
	if err := pc.Connect(); err != nil {
		return fmt.Errorf("failed to connect to printer: %v", err)
	}

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
	log.Printf("Message: %s", job.Message)

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

	// Print image if specified
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
				log.Printf("Image printed successfully")
				pc.printer.LineFeed()
			}
		}
	}

	// Reset formatting
	pc.printer.Bold(false)
	pc.printer.Underline(0)
	pc.printer.Justify(escpos.JustifyLeft)
	pc.printer.Size(1, 1)

	// Send to printer and cut
	if err := pc.printer.PrintAndCut(); err != nil {
		log.Printf("Error during PrintAndCut: %v", err)
		return fmt.Errorf("failed to send data to printer: %v", err)
	}

	log.Printf("Print job completed successfully")
	return nil
}

// MQTTClient handles MQTT communication
type MQTTClient struct {
	client  mqtt.Client
	config  Config
	printer *PrinterClient
}

// NewMQTTClient creates a new MQTT client
func NewMQTTClient(config Config, printer *PrinterClient) *MQTTClient {
	return &MQTTClient{
		config:  config,
		printer: printer,
	}
}

// Connect establishes MQTT connection
func (mc *MQTTClient) Connect() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(mc.config.MQTTBroker)
	opts.SetUsername(mc.config.MQTTUsername)
	opts.SetPassword(mc.config.MQTTPassword)
	opts.SetClientID("mqtt2escpos-" + fmt.Sprintf("%d", time.Now().Unix()))
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)

	mc.client = mqtt.NewClient(opts)

	if token := mc.client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %v", token.Error())
	}

	log.Printf("Connected to MQTT broker at %s", mc.config.MQTTBroker)
	return nil
}

// Subscribe subscribes to the print topic
func (mc *MQTTClient) Subscribe() error {
	token := mc.client.Subscribe(mc.config.MQTTTopic, 0, mc.messageHandler)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %v", mc.config.MQTTTopic, token.Error())
	}

	log.Printf("Subscribed to MQTT topic: %s", mc.config.MQTTTopic)
	return nil
}

// messageHandler processes incoming MQTT messages
func (mc *MQTTClient) messageHandler(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received message on topic %s: %s", msg.Topic(), string(msg.Payload()))

	var job PrintJob
	if err := json.Unmarshal(msg.Payload(), &job); err != nil {
		log.Printf("Error parsing JSON message: %v", err)
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
		mc.client.Disconnect(250)
	}
}

// getConfig loads configuration from environment variables
func getConfig() Config {
	return Config{
		MQTTBroker:     getEnv("MQTT_BROKER", "localhost:1883"),
		MQTTUsername:   getEnv("MQTT_USERNAME", ""),
		MQTTPassword:   getEnv("MQTT_PASSWORD", ""),
		MQTTTopic:      getEnv("MQTT_TOPIC", "printer/commands"),
		PrinterHost:    getEnv("PRINTER_HOST", "192.168.7.25"),
		PrinterPort:    getEnv("PRINTER_PORT", "9100"),
		PrinterProfile: getEnv("PRINTER_PROFILE", "TM-T88VI"),
		ImageTimeout:   30 * time.Second,
		MaxImageSize:   1024 * 1024, // 1MB
	}
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

	// Create printer client
	printer := NewPrinterClient(config)

	// Connect to printer
	if err := printer.Connect(); err != nil {
		log.Fatalf("Failed to connect to printer: %v", err)
	}
	defer printer.Close()

	// Create MQTT client
	mqttClient := NewMQTTClient(config, printer)

	// Connect to MQTT broker
	if err := mqttClient.Connect(); err != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", err)
	}
	defer mqttClient.Disconnect()

	// Subscribe to print topic
	if err := mqttClient.Subscribe(); err != nil {
		log.Fatalf("Failed to subscribe to MQTT topic: %v", err)
	}

	log.Println("MQTT to ESC/POS bridge is running. Press Ctrl+C to exit.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
}
