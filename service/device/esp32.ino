#include <WiFi.h>
#include <PubSubClient.h>
#include <IRremoteESP8266.h>
#include <IRsend.h>
#include <ir_Daikin.h> // NOTE: Change this library to match the actual AC brand used (e.g., ir_Samsung.h, ir_LG.h)
#include <PZEM004Tv30.h>
#include <Wire.h>
#include <Adafruit_Sensor.h>
#include "Adafruit_BME680.h"
#include "time.h"

const char* DEVICE_ID     = "node_1";
// node 2 device ID
// const char* DEVICE_ID     = "node_2";

const uint16_t IR_PIN     = 4;
#define PZEM_RX 16
#define PZEM_TX 17

const char* ssid          = "smartliving";
const char* password      = "SmartLiving#123";
const char* mqtt_server   = "10.100.1.218";
const int   mqtt_port     = 1883;
const char* mqtt_user     = "smartliving";
const char* mqtt_pass     = "claudemonet";

// node 1 topic
const char* topic_cmd     = "smarthome/ac/node_1/command";
const char* topic_sensor  = "smarthome/ac/node_1/sensors";
// node 2 topic
// const char* topic_cmd     = "smarthome/ac/node_2/command";
// const char* topic_sensor  = "smarthome/ac/node_2/sensors";

WiFiClient espClient;
PubSubClient mqttClient(espClient);
IRDaikinESP ac(IR_PIN);
PZEM004Tv30 pzem(Serial2, PZEM_RX, PZEM_TX);
Adafruit_BME680 bme;

unsigned long lastSensorRead = 0;
const unsigned long sensorInterval = 5000; 

const char* ntpServer       = "pool.ntp.org";
const long  gmtOffset_sec   = 7 * 3600;
const int   daylightOffset_sec = 0;

String getFormattedTime() {
    struct tm timeinfo;
    if (!getLocalTime(&timeinfo)) {
        return "N/A";
    }
    char timeStringBuff[30];
    // Format: YYYY-MM-DD HH:MM:SS
    strftime(timeStringBuff, sizeof(timeStringBuff), "%Y-%m-%d %H:%M:%S", &timeinfo);
    return String(timeStringBuff);
}

void mqttCallback(char* topic, byte* payload, unsigned int length) {
    String message = "";
    for (int i = 0; i < length; i++) {
        message += (char)payload[i];
    }
    
    Serial.print("MQTT Received [node 1]: ");
    Serial.println(message);

    if (message == "ON") {
        ac.on(); ac.send();
    } else if (message == "OFF") {
        ac.off(); ac.send();
    } else if (message == "COOL24") {
        ac.on(); ac.setMode(kDaikinCool); ac.setTemp(24); ac.setFan(kDaikinFanAuto); ac.send();
    }
}

void reconnectMQTT() {
    while (!mqttClient.connected()) {
        Serial.print("Connecting MQTT Broker (node 1)...");
        String clientId = "ESP32_AC_Client_node1";
        
        if (mqttClient.connect(clientId.c_str(), mqtt_user, mqtt_pass)) {
            Serial.println(" Connected!");
            mqttClient.subscribe(topic_cmd);
        } else {
            Serial.print(" Failed, rc=");
            Serial.println(mqttClient.state());
            delay(5000);
        }
    }
}

void setup() {
    Serial.begin(115200);

    ac.begin(); ac.off();
    
    if (!bme.begin()) Serial.println("BME680 node 1 not found");
    else {
        bme.setTemperatureOversampling(BME680_OS_8X);
        bme.setHumidityOversampling(BME680_OS_2X);
        bme.setPressureOversampling(BME680_OS_4X);
        bme.setIIRFilterSize(BME680_FILTER_SIZE_3);
        bme.setGasHeater(320, 150);
    }

    WiFi.mode(WIFI_STA);
    WiFi.begin(ssid, password);
    while (WiFi.status() != WL_CONNECTED) {
        delay(500);
        Serial.print(".");
    }
    Serial.println("\nWiFi connected (node 1). IP: " + WiFi.localIP().toString());
    configTime(gmtOffset_sec, daylightOffset_sec, ntpServer);

    mqttClient.setServer(mqtt_server, mqtt_port);
    mqttClient.setCallback(mqttCallback);
}

void loop() {
    if (!mqttClient.connected()) reconnectMQTT();
    mqttClient.loop();

    if (millis() - lastSensorRead >= sensorInterval) {
        lastSensorRead = millis();

        String currentTime = getFormattedTime();

        float temp = 0, hum = 0, press = 0, gas = 0, approx_altitude = 0;
        if (bme.performReading()) {
            temp  = bme.temperature;
            hum   = bme.humidity;
            press = bme.pressure / 100.0;
            gas   = bme.gas_resistance;
            approx_altitude = bme.readAltitude(SENSORS_PRESSURE_SEALEVELHPA);
        }

        float volt    = pzem.voltage();
        float curr    = pzem.current();
        float power   = pzem.power();
        float energy  = pzem.energy();

        String jsonPayload = "{";
        jsonPayload += "\"device_id\":\"" + String(DEVICE_ID) + "\",";
        jsonPayload += "\"timestamp\":\"" + currentTime + "\",";
        jsonPayload += "\"temp\":" + String(isnan(temp) ? 0 : temp, 1) + ",";
        jsonPayload += "\"humidity\":" + String(isnan(hum) ? 0 : hum, 1) + ",";
        jsonPayload += "\"pressure\":" + String(isnan(press) ? 0 : press, 1) + ",";
        jsonPayload += "\"gas\":" + String(isnan(gas) ? 0 : gas, 1) + ",";
        jsonPayload += "\"approx_altitude\":" + String(isnan(approx_altitude) ? 0 : approx_altitude, 1) + ",";
        jsonPayload += "\"voltage\":" + String(isnan(volt) ? 0 : volt, 1) + ",";
        jsonPayload += "\"current\":" + String(isnan(curr) ? 0 : curr, 3) + ",";
        jsonPayload += "\"power\":" + String(isnan(power) ? 0 : power, 1) + ",";
        jsonPayload += "\"energy\":" + String(isnan(energy) ? 0 : energy, 3);
        jsonPayload += "}";

        mqttClient.publish(topic_sensor, jsonPayload.c_str());
        Serial.println("Published [node 1]: " + jsonPayload);
    }
}