package main

// Printer status polling and Home Assistant MQTT discovery.

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

const (
	topicBridgeStatus  = "mqtt2escpos/status"  // "online" / "offline" (retained, also the MQTT will)
	topicPrinterStatus = "mqtt2escpos/printer" // JSON PrinterStatus (retained)
	discoveryPrefix    = "homeassistant"
	deviceID           = "mqtt2escpos"
)

// PrinterStatus is what we know about the printer. Pointer fields are nil when
// the printer is reachable but didn't answer the status query.
type PrinterStatus struct {
	Online    bool  `json:"online"`
	PaperLow  *bool `json:"paper_low"`
	PaperOut  *bool `json:"paper_out"`
	CoverOpen *bool `json:"cover_open"`
}

// ESC/POS real-time status request: DLE EOT n.
const (
	statusOfflineCause = 2 // bit 2: cover open, bit 5: stopped because paper ran out
	statusRollPaper    = 4 // bits 2-3: paper near end, bits 5-6: paper end
)

// queryRealtimeStatus sends DLE EOT n and returns the single status byte.
func queryRealtimeStatus(conn net.Conn, n byte) (byte, error) {
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte{0x10, 0x04, n}); err != nil {
		return 0, err
	}
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != nil {
		return 0, err
	}
	// Every DLE EOT response has bit 1 and bit 4 set, bit 0 and bit 7 clear.
	if buf[0]&0x93 != 0x12 {
		return 0, fmt.Errorf("unexpected status byte 0x%02x", buf[0])
	}
	return buf[0], nil
}

// QueryStatus opens a short connection to the printer and asks for its status.
func (pc *PrinterClient) QueryStatus() PrinterStatus {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	conn, err := net.DialTimeout("tcp", pc.addr(), 3*time.Second)
	if err != nil {
		return PrinterStatus{Online: false}
	}
	defer conn.Close()

	st := PrinterStatus{Online: true}
	if b, err := queryRealtimeStatus(conn, statusRollPaper); err == nil {
		low, out := b&0x0C != 0, b&0x60 != 0
		st.PaperLow, st.PaperOut = &low, &out
	} else {
		log.Printf("Printer did not answer paper status query: %v", err)
		return st
	}
	if b, err := queryRealtimeStatus(conn, statusOfflineCause); err == nil {
		cover := b&0x04 != 0
		st.CoverOpen = &cover
	}
	return st
}

// requestStatusRefresh asks the poller to check the printer soon (non-blocking).
func (mc *MQTTClient) requestStatusRefresh() {
	select {
	case mc.refresh <- struct{}{}:
	default:
	}
}

// runStatusPoller publishes the printer status on a timer, after every job,
// and whenever we (re)connect to the broker.
func (mc *MQTTClient) runStatusPoller() {
	ticker := time.NewTicker(mc.config.StatusInterval)
	defer ticker.Stop()
	var last string
	for {
		select {
		case <-ticker.C:
		case <-mc.refresh:
			time.Sleep(time.Second) // let the printer settle after a job
		}
		if !mc.client.IsConnectionOpen() {
			continue
		}
		st := mc.printer.QueryStatus()
		payload, _ := json.Marshal(st)
		if string(payload) != last {
			log.Printf("Printer status: %s", payload)
			last = string(payload)
		}
		mc.publish(topicPrinterStatus, string(payload), true)
	}
}

// publish sends a QoS 1 message and waits briefly for it to go out.
func (mc *MQTTClient) publish(topic, payload string, retained bool) {
	t := mc.client.Publish(topic, 1, retained, payload)
	if !t.WaitTimeout(5*time.Second) || t.Error() != nil {
		log.Printf("Failed to publish to %s: %v", topic, t.Error())
	}
}

// publishDiscovery registers the bridge's entities with Home Assistant.
func (mc *MQTTClient) publishDiscovery() {
	device := map[string]any{
		"identifiers":  []string{deviceID},
		"name":         "Receipt Printer",
		"model":        mc.printer.profile.Name,
		"manufacturer": "ESC/POS",
		"sw_version":   mc.config.Version,
	}
	availability := []map[string]string{{"topic": topicBridgeStatus}}

	// Binary sensor template that maps true/false/null to ON/OFF/unknown.
	tristate := func(field string) string {
		return fmt.Sprintf("{%% if value_json.%[1]s is none %%}None{%% elif value_json.%[1]s %%}ON{%% else %%}OFF{%% endif %%}", field)
	}

	entities := []struct {
		component, id string
		config        map[string]any
	}{
		{"binary_sensor", "bridge", map[string]any{
			"name":            "Bridge",
			"state_topic":     topicBridgeStatus,
			"payload_on":      "online",
			"payload_off":     "offline",
			"device_class":    "connectivity",
			"entity_category": "diagnostic",
		}},
		{"binary_sensor", "printer", map[string]any{
			"name":           "Printer",
			"state_topic":    topicPrinterStatus,
			"value_template": "{{ 'ON' if value_json.online else 'OFF' }}",
			"device_class":   "connectivity",
			"availability":   availability,
		}},
		{"binary_sensor", "paper_low", map[string]any{
			"name":           "Paper low",
			"state_topic":    topicPrinterStatus,
			"value_template": tristate("paper_low"),
			"device_class":   "problem",
			"icon":           "mdi:paper-roll-outline",
			"availability":   availability,
		}},
		{"binary_sensor", "paper_out", map[string]any{
			"name":           "Paper out",
			"state_topic":    topicPrinterStatus,
			"value_template": tristate("paper_out"),
			"device_class":   "problem",
			"icon":           "mdi:paper-roll",
			"availability":   availability,
		}},
		{"binary_sensor", "cover_open", map[string]any{
			"name":           "Cover open",
			"state_topic":    topicPrinterStatus,
			"value_template": tristate("cover_open"),
			"device_class":   "problem",
			"availability":   availability,
		}},
		{"button", "feed", map[string]any{
			"name":          "Feed",
			"command_topic": mc.config.MQTTTopic,
			"payload_press": `{"command":"feed","lines":3}`,
			"qos":           1,
			"icon":          "mdi:arrow-down-bold",
			"availability":  availability,
		}},
		{"button", "cut", map[string]any{
			"name":          "Cut",
			"command_topic": mc.config.MQTTTopic,
			"payload_press": `{"command":"cut"}`,
			"qos":           1,
			"icon":          "mdi:content-cut",
			"availability":  availability,
		}},
		{"button", "feed_and_cut", map[string]any{
			"name":          "Feed and cut",
			"command_topic": mc.config.MQTTTopic,
			"payload_press": `{"command":"feed_and_cut","lines":3}`,
			"qos":           1,
			"icon":          "mdi:receipt-text-outline",
			"availability":  availability,
		}},
	}

	for _, e := range entities {
		e.config["unique_id"] = deviceID + "_" + e.id
		e.config["device"] = device
		payload, _ := json.Marshal(e.config)
		topic := fmt.Sprintf("%s/%s/%s/%s/config", discoveryPrefix, e.component, deviceID, e.id)
		mc.publish(topic, string(payload), true)
	}
	log.Printf("Published Home Assistant discovery for %d entities", len(entities))
}
