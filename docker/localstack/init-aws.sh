#!/bin/bash
# Creates the local equivalents of the production queues, topics, streams and buckets (container-design §4).
set -euo pipefail
R=ap-southeast-5

awslocal sns create-topic --region $R --name ac-domain-events.fifo \
  --attributes FifoTopic=true,ContentBasedDeduplication=false >/dev/null

for q in command-requested device-ack device-event notification-requested importexport-jobs; do
  awslocal sqs create-queue --region $R --queue-name ${q}-dlq.fifo --attributes FifoQueue=true >/dev/null
  DLQ_ARN=$(awslocal sqs get-queue-attributes --region $R --queue-url http://localhost:4566/000000000000/${q}-dlq.fifo \
    --attribute-names QueueArn --query Attributes.QueueArn --output text)
  awslocal sqs create-queue --region $R --queue-name ${q}.fifo \
    --attributes "{\"FifoQueue\":\"true\",\"VisibilityTimeout\":\"60\",\"RedrivePolicy\":\"{\\\"deadLetterTargetArn\\\":\\\"${DLQ_ARN}\\\",\\\"maxReceiveCount\\\":\\\"5\\\"}\"}" >/dev/null
done

awslocal kinesis create-stream --region $R --stream-name ac-telemetry --shard-count 2
for b in ac-files ac-reports ac-telemetry-archive ac-audit-export; do
  awslocal s3 mb s3://$b --region $R >/dev/null
done
awslocal ses verify-email-identity --region $R --email-address no-reply@ac.local
awslocal secretsmanager create-secret --region $R --name ac/local/stripe --secret-string '{"secretKey":"sk_test_local","webhookSecret":"whsec_local"}' >/dev/null
echo "AC local AWS resources ready"
