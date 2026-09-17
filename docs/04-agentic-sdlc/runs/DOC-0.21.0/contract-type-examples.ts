// Static acceptance-input DTO checks only; not an application test.

import type { OperationContracts, NotificationType, ChangeEntityType } from '../../../02-design/service-contracts';

const policyInput = {
  "name": "Demo CO2 A12",
  "unitIds": [
    "unit-online-rto",
    "unit-non-rto"
  ],
  "timezone": "Asia/Kuala_Lumpur",
  "enabled": true,
  "priority": 50,
  "kind": "air_quality",
  "metric": "co2",
  "operator": "gte",
  "threshold": 1000,
  "recoveryThreshold": 900,
  "durationSeconds": 60,
  "responseMode": "notify_and_ventilate",
  "ventilationLevel": "low",
  "recipientMembershipIds": [
    "hq-operator"
  ],
  "severity": "warning",
  "channels": [
    "inApp"
  ],
  "cooldownMinutes": 5,
  "escalateAfterMinutes": 60
} satisfies OperationContracts['policies.save']['input'];

const startEvaluation = {
  "eventId": "evt-a12-1",
  "occurredAt": "2026-09-14T01:00:00.000Z",
  "unitIds": [
    "unit-online-rto",
    "unit-non-rto"
  ],
  "facts": [
    {
      "unitId": "unit-online-rto",
      "metric": "co2",
      "value": 1100,
      "unit": "ppm",
      "observedAt": "2026-09-14T01:00:00.000Z",
      "quality": "valid"
    },
    {
      "unitId": "unit-non-rto",
      "metric": "co2",
      "value": 1100,
      "unit": "ppm",
      "observedAt": "2026-09-14T01:00:00.000Z",
      "quality": "valid"
    }
  ],
  "phase": "condition"
} satisfies OperationContracts['automations.fire']['input'];

const finalEvaluation = {
  "eventId": "evt-a12-at-60",
  "occurredAt": "2026-09-14T01:01:00.000Z",
  "unitIds": [
    "unit-online-rto",
    "unit-non-rto"
  ],
  "facts": [
    {
      "unitId": "unit-online-rto",
      "metric": "co2",
      "value": 1100,
      "unit": "ppm",
      "observedAt": "2026-09-14T01:00:00.000Z",
      "quality": "valid"
    },
    {
      "unitId": "unit-non-rto",
      "metric": "co2",
      "value": 1100,
      "unit": "ppm",
      "observedAt": "2026-09-14T01:00:00.000Z",
      "quality": "valid"
    }
  ],
  "phase": "condition"
} satisfies OperationContracts['automations.fire']['input'];

const loadTrigger = {
  "scenarioId": "AT-G121-002",
  "eventId": "evt-g121-load-1",
  "occurredAt": "2026-09-14T01:00:00.000Z",
  "eventType": "load_alert",
  "unitId": "unit-online-rto",
  "causeCode": "insulation_loss",
  "evidenceKind": "inferred",
  "evidenceText": "Demo insulation",
  "severity": "warning"
} satisfies OperationContracts['demo.trigger']['input'];

const allergenTrigger = {
  "scenarioId": "AT-G121-004",
  "eventId": "evt-g121-allergen-1",
  "occurredAt": "2026-09-14T01:00:00.000Z",
  "eventType": "allergen",
  "observation": {
    "unitId": "unit-online-rto",
    "availability": "available",
    "substance": "Demo allergen",
    "value": 20,
    "unit": "ng/m³",
    "sourceLabel": "Demo",
    "observedAt": "2026-09-14T01:00:00.000Z",
    "evidenceText": "Demo observation"
  }
} satisfies OperationContracts['demo.trigger']['input'];

const faultType = 'fault' satisfies NotificationType;

const changeType = 'allergen_observation' satisfies ChangeEntityType;
