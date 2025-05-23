#!/bin/bash
# This script installs dependencies and builds the React application.
# Run it from the 'webui/frontend' directory.

echo "Installing dependencies..."
npm install

echo "Building the application..."
# Use 'npm run build' if you have a 'build' script in package.json configured for Vite
# Otherwise, use 'npx vite build' directly.
npm run build
# OR, if you don't have a specific build script:
# npx vite build

echo "Build complete. Artifacts should be in the 'dist' directory."
