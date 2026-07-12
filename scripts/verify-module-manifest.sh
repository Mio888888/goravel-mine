#!/usr/bin/env sh
set -eu

go run . artisan module:manifest:check --artifacts --frontend
