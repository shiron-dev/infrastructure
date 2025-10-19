SCRIPTS_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
ROOT_DIR := $(abspath $(SCRIPTS_DIR)/..)
VENV_DIR := $(SCRIPTS_DIR)/.venv
PYTHON := $(VENV_DIR)/bin/python
PIP := $(VENV_DIR)/bin/pip

SERVICES_DIR := $(SCRIPTS_DIR)/my-services
SERVICES_YAML := $(ROOT_DIR)/data/my-services/services.yml
SERVICES_SCHEMA := $(SERVICES_DIR)/services.schema.json
CONVERTER := $(SERVICES_DIR)/convert_to_homer.py

.PHONY: help
help:
	@echo "Usage: make -f scripts/Makefile [target]"
	@echo ""
	@echo "Targets:"
	@echo "  setup               Create venv and install dependencies"
	@echo "  homer-yaml          Generate Homer YAML config"
	@echo "  validate-services   Validate services.yml with JSON schema"
	@echo "  clean-venv          Remove venv"

.PHONY: venv
venv:
	@if [ ! -d "$(VENV_DIR)" ]; then python3 -m venv $(VENV_DIR); fi

.PHONY: install
install: venv
	$(PIP) install --upgrade pip
	$(PIP) install pyyaml jsonschema

.PHONY: setup
setup: install

.PHONY: homer-yaml
homer-yaml: setup
	$(PYTHON) $(CONVERTER) --services $(SERVICES_YAML) --schema $(SERVICES_SCHEMA) --output $(ROOT_DIR)/data/my-services/homer_config.yml

.PHONY: validate-services
validate-services: setup
	$(PYTHON) $(CONVERTER) --services $(SERVICES_YAML) --schema $(SERVICES_SCHEMA) --validate-only

.PHONY: clean-venv
clean-venv:
	rm -rf $(VENV_DIR)

.PHONY: all
all: homer-yaml

.DEFAULT_GOAL := help

