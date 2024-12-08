# Makefile for GoCart SDK TypeScript Generator

# Variables
BIN_DIR := bin
EXECUTABLE := $(BIN_DIR)/gocart-sdk-ts-gen
SRC_DIR := .
SHELL_CONFIG := $(HOME)/.zshrc

.PHONY: all build clean help

# Default target
all: build

# Build the tool and place the executable in the bin directory
build:
	@echo "Building the tool..."
	@mkdir -p $(BIN_DIR)
	@go build -o $(EXECUTABLE) $(SRC_DIR)
	@echo "Executable created at $(EXECUTABLE)"
	@if ! grep -q "$(BIN_DIR)" $(SHELL_CONFIG); then \
		echo "Adding 'bin' directory to PATH in $(SHELL_CONFIG)"; \
		echo "export PATH=\$$PATH:$(shell pwd)/$(BIN_DIR)" >> $(SHELL_CONFIG); \
		echo "Run 'source $(SHELL_CONFIG)' or restart your terminal to apply changes."; \
	else \
		echo "'bin' directory is already in PATH in $(SHELL_CONFIG)"; \
	fi

# Clean the bin directory
clean:
	@echo "Cleaning up..."
	@rm -rf $(BIN_DIR)
	@echo "Clean complete."

# Help message
help:
	@echo "Makefile for GoCart SDK TypeScript Generator"
	@echo
	@echo "Usage:"
	@echo "  make build      Build the tool and place the executable in the bin directory."
	@echo "  make clean      Remove all build artifacts."
	@echo "  make help       Show this help message."
