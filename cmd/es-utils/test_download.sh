#!/bin/bash

set -e

txt_files=(./compare/*.txt)
for txt_file in "${txt_files[@]}"; do
    base_name=$(basename "$txt_file")
    dat_file="./compare/${base_name%.*}.dat"

    if [[ -f "$dat_file" ]]; then
        if cmp -s "$txt_file" "$dat_file"; then
            echo "Content matches for $txt_file and $dat_file"
            rm "$txt_file" "$dat_file"
        else
            echo "Content differs for $txt_file and $dat_file"
            exit
        fi
    else
        echo "Corresponding .dat file not found for $txt_file"
        exit
    fi
done
echo "All tests passed"
