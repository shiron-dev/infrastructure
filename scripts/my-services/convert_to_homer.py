#!/usr/bin/env python3

import yaml
import json
import argparse
import sys
from pathlib import Path
from typing import Dict, List, Any
from jsonschema import validate, ValidationError


def load_yaml_file(file_path: str) -> Dict[str, Any]:
    try:
        with open(file_path, 'r', encoding='utf-8') as file:
            return yaml.safe_load(file)
    except FileNotFoundError:
        print(f"Error: File '{file_path}' not found.")
        sys.exit(1)
    except yaml.YAMLError as e:
        print(f"Error: Failed to parse YAML file: {e}")
        sys.exit(1)


def load_json_schema(file_path: str) -> Dict[str, Any]:
    try:
        with open(file_path, 'r', encoding='utf-8') as file:
            return json.load(file)
    except FileNotFoundError:
        print(f"Error: File '{file_path}' not found.")
        sys.exit(1)
    except json.JSONDecodeError as e:
        print(f"Error: Failed to parse JSON file: {e}")
        sys.exit(1)


def validate_data(data: Dict[str, Any], schema: Dict[str, Any]) -> bool:
    try:
        validate(instance=data, schema=schema)
        print("Data conforms to the schema.")
        return True
    except ValidationError as e:
        print(f"Validation error: {e.message}")
        print(f"   Path: {' -> '.join(str(p) for p in e.absolute_path)}")
        return False


def to_homer_items(services: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    items: List[Dict[str, Any]] = []
    for service in services:
        subtitle = service.get('description')
        icon = service.get('image')
        item: Dict[str, Any] = {
            'name': service['name'],
            'url': service['url'],
        }
        if subtitle:
            item['subtitle'] = subtitle
        if icon:
            item['logo'] = icon
        # open in same tab by default; users can change to _blank if desired
        item['_target'] = 'self'
        items.append(item)
    return items


def convert_to_homer_config(services_data: Dict[str, Any]) -> Dict[str, Any]:
    services_list: List[Dict[str, Any]] = services_data.get('services', [])

    homer_config: Dict[str, Any] = {
        'title': 'My Services',
        'subtitle': 'オンプレミスサービス一覧',
        'theme': 'default',
        'message': '',
        'links': [],
        'services': [
            {
                'name': 'Services',
                'icon': 'fas fa-th',
                'items': to_homer_items(services_list),
            }
        ],
    }

    return homer_config


def write_homer_yaml(homer_config: Dict[str, Any], output_path: str) -> None:
    try:
        with open(output_path, 'w', encoding='utf-8') as file:
            yaml.dump(homer_config, file, default_flow_style=False, allow_unicode=True, sort_keys=False)
        print(f"Generated Homer config: {output_path}")
    except Exception as e:
        print(f"Failed to save file: {e}")
        sys.exit(1)


def main():
    parser = argparse.ArgumentParser(
        description='Convert services.yml to Homer dashboard configuration'
    )
    parser.add_argument(
        '--services',
        default='data/my-services/services.yml',
        help='Path to services.yml (default: data/my-services/services.yml)'
    )
    parser.add_argument(
        '--schema',
        default='scripts/my-services/services.schema.json',
        help='Path to JSON Schema (default: scripts/my-services/services.schema.json)'
    )
    parser.add_argument(
        '--output',
        default='data/my-services/homer_config.yml',
        help='Output file path (default: data/my-services/homer_config.yml)'
    )
    parser.add_argument(
        '--no-validate',
        action='store_true',
        help='Skip schema validation'
    )
    parser.add_argument(
        '--validate-only',
        action='store_true',
        help='Validate only; do not generate output'
    )

    args = parser.parse_args()

    print("Starting Homer configuration conversion...")
    print(f"   Input file: {args.services}")
    print(f"   Schema file: {args.schema}")
    print(f"   Output file: {args.output}")

    if not Path(args.services).exists():
        print(f"Error: File '{args.services}' does not exist.")
        sys.exit(1)

    if not Path(args.schema).exists():
        print(f"Error: File '{args.schema}' does not exist.")
        sys.exit(1)

    print("\nLoading services.yml...")
    services_data = load_yaml_file(args.services)

    print("Loading JSON Schema...")
    schema = load_json_schema(args.schema)

    if not args.no_validate:
        print("\nValidating data...")
        if not validate_data(services_data, schema):
            print("Validation failed. Aborting.")
            sys.exit(1)
    else:
        print("Skipping validation.")

    if args.validate_only:
        print("\nValidation-only run. No conversion performed.")
        sys.exit(0)

    print("\nConverting to Homer config format...")
    homer_config = convert_to_homer_config(services_data)

    print(f"\nWriting configuration file...")
    write_homer_yaml(homer_config, args.output)

    print(f"\nConversion completed!")
    print(f"   Services converted: {len(homer_config.get('services', [{}])[0].get('items', []))}")
    print(f"   Output file: {args.output}")


if __name__ == "__main__":
    main()




















