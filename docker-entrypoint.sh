#!/bin/sh

# Ensure the SQLite data directory exists
mkdir -p /app/data

# Execute the main binary
exec ./qradar-collector
