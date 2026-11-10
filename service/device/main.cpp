#include <Arduino.h>
#include <WiFi.h>
#include <PubSubClient.h>
#include <IRremoteESP8266.h>
#include <IRsend.h>
#include <ir_Daikin.h>
#include <PZEM004Tv30.h>
#include <Wire.h>
#include <Adafruit_Sensor.h>
#include "Adafruit_BME680.h"
#include "DFRobot_AirQualitySensor.h"
#include "time.h"
#include <vector>
#include <ArduinoJson.h>
#include <Preferences.h>

#include "config.h" 

#define FIRMWARE_VERSION "0.4.0" // Versi firmware untuk heartbeat

const uint16_t IR_PIN = 4;
#define PZEM_RX 16
#define PZEM_TX 17
#define SDA_PIN 21
#define SCL_PIN 22
#define PM_I2C_ADDRESS 0x19

char topic_cmd[64];
char topic_result[64];
char topic_sensor[64];
char topic_status[64];
char topic_state[64];
char mqtt_client_id[32];

WiFiClient espClient;
PubSubClient mqttClient(espClient);

IRDaikinESP ac(IR_PIN);
PZEM004Tv30 pzem(Serial2, PZEM_RX, PZEM_TX);
Adafruit_BME680 bme;
DFRobot_AirQualitySensor particle(&Wire, PM_I2C_ADDRESS);
Preferences acPrefs;

// Timing Sensor
unsigned long lastSensorRead = 0;
const unsigned long sensorInterval = 5000; 

// Timing Heartbeat (DEV-20)
unsigned long lastHeartbeat = 0;
const unsigned long heartbeatInterval = 60000; 

// Reconnect Mechanism
unsigned long lastReconnectAttempt = 0;
unsigned int reconnectDelay = 1000; 
const unsigned int maxReconnectDelay = 30000; 

std::vector<String> offlineBuffer;
const int MAX_BUFFER_SIZE = 50; 

bool bme_connected = false;
bool pm_connected = false;

// Variabel State AC
bool ac_power = false;
uint8_t ac_mode = kDaikinCool;
uint8_t ac_temp = 24;
uint8_t ac_fan = kDaikinFanAuto;

unsigned long lastCommandTime = 0;

const char* ntpServer = "pool.ntp.org";
const long gmtOffset_sec = 7 * 3600;
const int daylightOffset_sec = 0;

String getFormattedTime() {
  struct tm timeinfo;
  if (!getLocalTime(&timeinfo)) return "N/A";
  char timeStringBuff[35];
  strftime(timeStringBuff, sizeof(timeStringBuff), "%Y-%m-%dT%H:%M:%S+07:00", &timeinfo);
  return String(timeStringBuff);
}

String getModeString(uint8_t mode) {
  if (mode == kDaikinCool) return "cool";
  if (mode == kDaikinDry) return "dry";
  if (mode == kDaikinFan) return "fan";
  if (mode == kDaikinAuto) return "auto";
  return "cool";
}

String getFanString(uint8_t fan) {
  if (fan == kDaikinFanAuto) return "auto";
  if (fan == 1) return "low";
  if (fan == 3) return "mid";
  if (fan == 5 || fan == 10) return "high"; 
  return "auto";
}

// ------------------- DEV-20/21: Fungsi Heartbeat -------------------
void sendHeartbeat() {
  StaticJsonDocument<256> doc;
  
  doc["online"] = true;
  doc["fw"] = FIRMWARE_VERSION;
  doc["ip"] = WiFi.localIP().toString();
  doc["rssi"] = WiFi.RSSI();
  doc["uptime_s"] = millis() / 1000;

  JsonObject parts = doc.createNestedObject("parts");
  
  parts["ir"] = (String(NODE_ID) != "node_2");
  parts["bme680"] = bme_connected;
  parts["pm25"] = pm_connected;
  
  float pzemVolt = pzem.voltage();
  parts["pzem"] = (!isnan(pzemVolt) && pzemVolt > 0.0);

  String payload;
  serializeJson(doc, payload);
  
  // Publish ke topic status dengan Retained flag = true
  mqttClient.publish(topic_status, payload.c_str(), true);
  
  lastHeartbeat = millis(); // Reset timer heartbeat
  Serial.println("Heartbeat sent: " + payload);
}
// -------------------------------------------------------------------

void sendCommandResult(String commandId, String rawCommand, String resultState) {
  StaticJsonDocument<512> resDoc;
  
  if (commandId == "null" || commandId.isEmpty()) resDoc["commandId"] = nullptr;
  else resDoc["commandId"] = commandId;
  
  resDoc["command"] = rawCommand;
  resDoc["result"] = resultState; 
  resDoc["at"] = getFormattedTime();

  String resStr;
  serializeJson(resDoc, resStr);
  mqttClient.publish(topic_result, resStr.c_str());
}

void mqttCallback(char* topic, byte* payload, unsigned int length) {
  String rawCommand = "";
  for (unsigned int i = 0; i < length; i++) rawCommand += (char)payload[i];
  
  String commandId = "null";
  String resultState = "sent";
  bool sendIR = false;

  StaticJsonDocument<256> doc;
  DeserializationError error = deserializeJson(doc, rawCommand);

  if (!error && doc.is<JsonObject>()) {
    if (doc.containsKey("commandId")) commandId = doc["commandId"].as<String>();

    bool invalid_found = false;
    
    if (doc.containsKey("power")) {
      String p = doc["power"];
      if (p == "ON") ac_power = true;
      else if (p == "OFF") ac_power = false;
      else invalid_found = true; 
    }
    
    if (doc.containsKey("temp")) {
      int t = doc["temp"];
      if (t >= 16 && t <= 30) ac_temp = t;
      else invalid_found = true; 
    }
    
    if (doc.containsKey("mode")) {
      String m = doc["mode"];
      if (m == "cool") ac_mode = kDaikinCool;
      else if (m == "dry") ac_mode = kDaikinDry;
      else if (m == "fan") ac_mode = kDaikinFan;
      else if (m == "auto") ac_mode = kDaikinAuto;
      else invalid_found = true; 
    }

    if (doc.containsKey("fan")) {
      String f = doc["fan"];
      if (f == "auto") ac_fan = kDaikinFanAuto;
      else if (f == "low") ac_fan = 1; 
      else if (f == "mid") ac_fan = 3; 
      else if (f == "high") ac_fan = 5; 
      else invalid_found = true; 
    }

    if (invalid_found) resultState = "invalid"; 
    else sendIR = true;

  } else {
    String cleanCmd = rawCommand;
    cleanCmd.trim();          
    cleanCmd.toUpperCase();   

    if (cleanCmd == "ON") {
      ac_power = true; sendIR = true;
    } else if (cleanCmd == "OFF") {
      ac_power = false; sendIR = true;
    } else if (cleanCmd == "COOL24") {
      ac_power = true; ac_mode = kDaikinCool; ac_temp = 24; ac_fan = kDaikinFanAuto;
      sendIR = true;
    } else {
      Serial.println("Unknown command: " + rawCommand);
      resultState = "unsupported"; 
    }
  }

  if (millis() - lastCommandTime < 1000) {
    resultState = "invalid"; 
    sendIR = false;
  }

//   if (String(NODE_ID) == "node_2") {
//     resultState = "unsupported";
//     sendIR = false;
//   }

  if (sendIR && resultState == "sent") {
    lastCommandTime = millis();

    acPrefs.putBool("power", ac_power);
    acPrefs.putUChar("temp", ac_temp);
    acPrefs.putUChar("mode", ac_mode);
    acPrefs.putUChar("fan", ac_fan);

    if (ac_power) {
      ac.on(); ac.setMode(ac_mode); ac.setTemp(ac_temp); ac.setFan(ac_fan);
    } else {
      ac.off();
    }
    ac.send(); 

    StaticJsonDocument<128> stateDoc;
    stateDoc["power"] = ac_power ? "ON" : "OFF";
    stateDoc["temp"] = ac_temp;
    stateDoc["mode"] = getModeString(ac_mode);
    stateDoc["fan"] = getFanString(ac_fan);
    
    String statePayload;
    serializeJson(stateDoc, statePayload);
    mqttClient.publish(topic_state, statePayload.c_str(), true); // true = Retained
  }

  sendCommandResult(commandId, rawCommand, resultState);
}

boolean connectMQTT() {
  Serial.printf("[%s] Connecting MQTT...\n", NODE_ID);
  
  // DEV-21: Last Will Config (Retained = true, QoS = 0)
  String willPayload = "{\"online\":false}";
  
  // Format connect: connect(clientId, user, pass, willTopic, willQoS, willRetain, willMessage)
  if (mqttClient.connect(mqtt_client_id, MQTT_USER, MQTT_PASS, topic_status, 0, true, willPayload.c_str())) {
    Serial.println("MQTT Connected!");
    
    // Subscribe kembali ke topic
    mqttClient.subscribe(topic_cmd);

    // DEV-20: Kirim Heartbeat segera setelah berhasil terhubung ke MQTT
    sendHeartbeat();

    // DEV-11: Publish AC State terakhir saat Boot
    StaticJsonDocument<128> bootDoc;
    bootDoc["power"] = ac_power ? "ON" : "OFF";
    bootDoc["temp"] = ac_temp;
    bootDoc["mode"] = ac_mode;
    bootDoc["fan"] = ac_fan;
    
    String bootPayload;
    serializeJson(bootDoc, bootPayload);
    
    mqttClient.publish(topic_state, bootPayload.c_str(), true);

    // Kirim antrean offline data sensor jika ada
    while (!offlineBuffer.empty() && mqttClient.connected()) {
      String bufferedMsg = offlineBuffer.front();
      if (mqttClient.publish(topic_sensor, bufferedMsg.c_str())) {
        Serial.println("Buffered data sent successfully.");
        offlineBuffer.erase(offlineBuffer.begin()); 
        mqttClient.loop();
        delay(50);
      } else {
        Serial.println("Failed to send buffered data. Halting queue.");
        break; 
      }
    }
    return true;
  }
  return false;
}

void setup() {
  Serial.begin(115200);

  acPrefs.begin("ac_state", false); 
  ac_power = acPrefs.getBool("power", false);
  ac_temp = acPrefs.getUChar("temp", 24);
  ac_mode = acPrefs.getUChar("mode", kDaikinCool);
  ac_fan = acPrefs.getUChar("fan", kDaikinFanAuto);

  snprintf(topic_cmd, sizeof(topic_cmd), "smarthome/ac/%s/command", NODE_ID);
  snprintf(topic_result, sizeof(topic_result), "smarthome/ac/%s/command/result", NODE_ID); 
  snprintf(topic_sensor, sizeof(topic_sensor), "smarthome/ac/%s/sensors", NODE_ID);
  snprintf(topic_status, sizeof(topic_status), "smarthome/ac/%s/status", NODE_ID); 
  snprintf(topic_state, sizeof(topic_state), "smarthome/ac/%s/state", NODE_ID);
  snprintf(mqtt_client_id, sizeof(mqtt_client_id), "ac-%s", NODE_ID);

  Wire.begin(SDA_PIN, SCL_PIN);
  ac.begin(); ac.off(); 
  
  if (bme.begin()) {
    bme_connected = true;
    bme.setTemperatureOversampling(BME680_OS_8X);
    bme.setHumidityOversampling(BME680_OS_2X);
    bme.setPressureOversampling(BME680_OS_4X);
    bme.setIIRFilterSize(BME680_FILTER_SIZE_3);
    bme.setGasHeater(320, 150);
  }

  if (particle.begin()) {
    pm_connected = true;
  }

  WiFi.mode(WIFI_STA);
  WiFi.begin(WIFI_SSID, WIFI_PASS);

  configTime(gmtOffset_sec, daylightOffset_sec, ntpServer);

  mqttClient.setServer(MQTT_SERVER, MQTT_PORT);
  mqttClient.setCallback(mqttCallback);
  mqttClient.setBufferSize(1024);
}

void loop() {
  if (!mqttClient.connected()) {
    unsigned long now = millis();
    if (now - lastReconnectAttempt > reconnectDelay) {
      lastReconnectAttempt = now;
      if (connectMQTT()) reconnectDelay = 1000; 
      else {
        reconnectDelay *= 2; 
        if (reconnectDelay > maxReconnectDelay) reconnectDelay = maxReconnectDelay; 
      }
    }
  } else {
    mqttClient.loop();
  }

  // DEV-20: Timer Heartbeat setiap 60 detik
  if (mqttClient.connected() && (millis() - lastHeartbeat >= heartbeatInterval)) {
    sendHeartbeat();
  }

  // Timer Data Sensor setiap 5 detik
  if (millis() - lastSensorRead >= sensorInterval) {
    lastSensorRead = millis();

    String jsonPayload = "{";
    jsonPayload += "\"node_id\":\"" + String(NODE_ID) + "\",";
    jsonPayload += "\"timestamp\":\"" + getFormattedTime() + "\""; 

    if (bme_connected && bme.performReading()) {
      jsonPayload += ",\"temp\":" + String(bme.temperature, 1);
      jsonPayload += ",\"humidity\":" + String(bme.humidity, 1);
      jsonPayload += ",\"pressure\":" + String(bme.pressure / 100.0, 1);
      jsonPayload += ",\"gas\":" + String(bme.gas_resistance);
      jsonPayload += ",\"approx_altitude\":" + String(bme.readAltitude(1013.25), 1); 
    }

    if (pm_connected) {
      uint16_t pm25 = particle.gainParticleConcentration_ugm3(PARTICLE_PM2_5_ATMOSPHERE);
      jsonPayload += ",\"pm25\":" + String(pm25);
    }

    float volt = pzem.voltage();
    if (!isnan(volt) && volt > 0.0) { 
      jsonPayload += ",\"voltage\":" + String(volt, 1);
      jsonPayload += ",\"current\":" + String(pzem.current(), 3);
      jsonPayload += ",\"power\":" + String(pzem.power(), 1);
      jsonPayload += ",\"energy\":" + String(pzem.energy(), 3);
    }

    jsonPayload += "}"; 

    if (mqttClient.connected()) {
      if (mqttClient.publish(topic_sensor, jsonPayload.c_str())) {
      } else {
        if (offlineBuffer.size() < MAX_BUFFER_SIZE) offlineBuffer.push_back(jsonPayload);
      }
    } else {
      if (offlineBuffer.size() < MAX_BUFFER_SIZE) offlineBuffer.push_back(jsonPayload);
    }
  }
}