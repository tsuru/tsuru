#!/bin/sh
echo "Job Started"

echo "Waiting 5 Seconds..."
sleep 5

if [ "$TEST_ENV" = "integration_test" ]; then
  echo "Environment variable TEST_ENV is set to integration_test"
fi

echo "DONE"
