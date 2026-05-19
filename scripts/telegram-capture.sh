#!/usr/bin/env bash

# Load environment variables if .env exists
if [ -f .env ]; then
    # shellcheck source=/dev/null
    source .env
fi

if [ -z "$TELEGRAM_BOT_TOKEN" ]; then
    echo "Error: TELEGRAM_BOT_TOKEN is not set."
    exit 1
fi

API_URL="https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN"
OFFSET_FILE=".offset"

# Get current offset
if [ -f "$OFFSET_FILE" ]; then
    OFFSET=$(cat "$OFFSET_FILE")
else
    OFFSET=0
fi

echo "Starting Telegram capture bot..."

while true; do
    # Long polling: timeout set to 60 seconds
    UPDATES=$(curl -s "$API_URL/getUpdates?offset=$OFFSET&timeout=60")
    
    # Process updates using jq
    # Iterates over each result in the 'result' array
    echo "$UPDATES" | jq -c '.result[]' | while read -r UPDATE; do
        UPDATE_ID=$(echo "$UPDATE" | jq '.update_id')
        MESSAGE=$(echo "$UPDATE" | jq -r '.message.text // empty')
        CHAT_ID=$(echo "$UPDATE" | jq '.message.chat.id')

        if [ -n "$MESSAGE" ]; then
            # Default routing
            PROFILE="personal"
            CLEAN_MESSAGE="$MESSAGE"

            # Check for prefixes
            if [[ "$MESSAGE" =~ ^/(w|work)\ + ]]; then
                PROFILE="work"
                CLEAN_MESSAGE=$(echo "$MESSAGE" | sed -E 's/^\/(w|work)\ +//')
            elif [[ "$MESSAGE" =~ ^/(p|personal)\ + ]]; then
                PROFILE="personal"
                CLEAN_MESSAGE=$(echo "$MESSAGE" | sed -E 's/^\/(p|personal)\ +//')
            fi

            # Prepare Logseq entry
            DATE=$(date +%Y_%m_%d)
            TIME=$(date +%H:%M)
            JOURNAL_DIR="$PROFILE/journals"
            JOURNAL_FILE="$JOURNAL_DIR/$DATE.md"

            mkdir -p "$JOURNAL_DIR"
            
            # Format: - [TIME] [Message]
            ENTRY="- $TIME $CLEAN_MESSAGE"
            
            echo "$ENTRY" >> "$JOURNAL_FILE"
            echo "Captured to $PROFILE journal: $CLEAN_MESSAGE"

            # Acknowledge (optional but helpful)
            curl -s -X POST "$API_URL/sendMessage" -d "chat_id=$CHAT_ID" -d "text=Captured to $PROFILE journal." > /dev/null
        fi

        # Update offset to next message
        OFFSET=$((UPDATE_ID + 1))
        echo "$OFFSET" > "$OFFSET_FILE"
    done

    # Small sleep to prevent tight loop in case of API errors
    sleep 1
done
