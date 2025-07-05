#!/usr/bin/with-contenv bashio

CONFIG_PATH=/data/options.json

if [ ! -f "$CONFIG_PATH" ]; then
    bashio::log.error "Configuration file not found: $CONFIG_PATH"
    exit 1
fi

MQTT_BROKER=$(bashio::config 'mqtt_broker')
MQTT_USERNAME=$(bashio::config 'mqtt_username')
MQTT_PASSWORD=$(bashio::config 'mqtt_password')
MQTT_TOPIC=$(bashio::config 'mqtt_topic')
PRINTER_HOST=$(bashio::config 'printer_host')
PRINTER_PORT=$(bashio::config 'printer_port')
PRINTER_PROFILE=$(bashio::config 'printer_profile')
IMAGE_TIMEOUT=$(bashio::config 'image_timeout')
MAX_IMAGE_SIZE=$(bashio::config 'max_image_size')

export MQTT_BROKER
export MQTT_USERNAME
export MQTT_PASSWORD
export MQTT_TOPIC
export PRINTER_HOST
export PRINTER_PORT
export PRINTER_PROFILE
export IMAGE_TIMEOUT
export MAX_IMAGE_SIZE

bashio::log.info "Starting MQTT to ESC/POS Printer Bridge"
bashio::log.info "MQTT Broker: $MQTT_BROKER"
bashio::log.info "MQTT Topic: $MQTT_TOPIC"
bashio::log.info "Printer: $PRINTER_HOST:$PRINTER_PORT"
bashio::log.info "Printer Profile: $PRINTER_PROFILE"

exec /usr/local/bin/mqtt2escpos 