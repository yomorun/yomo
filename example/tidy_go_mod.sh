#!/bin/bash

starting_dir="."

find_go_mod() {
    local dir="$1"
    
    for item in "$dir"/*; do
        if [ -d "$item" ]; then
            find_go_mod "$item"
        elif [ -f "$item" ] && [ "$item" == "$dir/go.mod" ]; then
            echo "Found go.mod in $dir, running 'go mod tidy'..."
            (cd "$dir" && go mod tidy)
        fi
    done
}

find_go_mod "$starting_dir"