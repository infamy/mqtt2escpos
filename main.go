package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
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

// Config holds the application configuration
type Config struct {
	MQTTBroker   string
	MQTTUsername string
	MQTTPassword string
	MQTTTopic    string
	PrinterHost  string
	PrinterPort  string
}

// PrinterClient handles the printer connection and printing
type PrinterClient struct {
	config  Config
	conn    net.Conn
	printer *escpos.Escpos
}

// NewPrinterClient creates a new printer client
func NewPrinterClient(config Config) *PrinterClient {
	return &PrinterClient{
		config: config,
	}
}

// Connect establishes connection to the printer
func (pc *PrinterClient) Connect() error {
	var err error
	pc.conn, err = net.Dial("tcp", pc.config.PrinterHost+":"+pc.config.PrinterPort)
	if err != nil {
		return fmt.Errorf("failed to connect to printer: %v", err)
	}

	pc.printer = escpos.New(pc.conn)
	pc.printer.SetConfig(escpos.ConfigEpsonTMT20II)

	log.Printf("Connected to printer at %s:%s", pc.config.PrinterHost, pc.config.PrinterPort)
	return nil
}

// Close closes the printer connection
func (pc *PrinterClient) Close() error {
	if pc.conn != nil {
		return pc.conn.Close()
	}
	return nil
}

// decodeImage decodes a base64 encoded image string
func (pc *PrinterClient) decodeImage(imageData, format string) (image.Image, error) {
	// Decode base64 data
	data, err := base64.StdEncoding.DecodeString(imageData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 image data: %v", err)
	}

	// Decode image based on format
	var img image.Image
	switch strings.ToLower(format) {
	case "png":
		img, err = png.Decode(strings.NewReader(string(data)))
	case "jpeg", "jpg":
		img, err = jpeg.Decode(strings.NewReader(string(data)))
	case "gif":
		img, err = gif.Decode(strings.NewReader(string(data)))
	default:
		return nil, fmt.Errorf("unsupported image format: %s", format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	return img, nil
}

// Print processes a print job
func (pc *PrinterClient) Print(job PrintJob) error {
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
	pc.printer.PrintAndCut()

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
		MQTTBroker:   getEnv("MQTT_BROKER", "localhost:1883"),
		MQTTUsername: getEnv("MQTT_USERNAME", ""),
		MQTTPassword: getEnv("MQTT_PASSWORD", ""),
		MQTTTopic:    getEnv("MQTT_TOPIC", "printer/commands"),
		PrinterHost:  getEnv("PRINTER_HOST", "192.168.7.25"),
		PrinterPort:  getEnv("PRINTER_PORT", "9100"),
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
