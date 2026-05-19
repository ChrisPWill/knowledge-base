#!/usr/bin/env bash

# Check if any files in journals/ or pages/ are tracked by Git (colocated with jj)
TRACKED_CONTENT=$(git ls-files '*/journals/*' '*/pages/*')

if [ -n "$TRACKED_CONTENT" ]; then
    echo "WARNING: Accidental tracking detected in content directories!"
    echo "$TRACKED_CONTENT"
    echo "Check your .gitignore and use 'jj abandon' or 'git rm --cached' to fix."
    exit 1
else
    echo "Health check passed: No content files are currently being tracked."
fi
