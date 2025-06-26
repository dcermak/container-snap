#!/bin/bash

set -euo pipefail

images_output=$(container-snap list-images 2>/dev/null || true)

if [ -z "$images_output" ]; then
    echo "No container images found in storage"
    exit 1
fi

# Parse the most recent image (assuming CSV format: ID,Created,Name)
# Sort by creation date and take the first one
most_recent_image=$(echo "$images_output" | sort -t',' -k2 -r | head -n1)

if [ -z "$most_recent_image" ]; then
    echo "Could not determine most recent image"
    exit 1
fi

# Extract image ID (first field)
image_id=$(echo "$most_recent_image" | cut -d',' -f1)

/usr/bin/container-snap-switch-snapshot.sh "$image_id"
