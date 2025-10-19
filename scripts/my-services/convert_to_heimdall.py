#!/usr/bin/env python3


import yaml
import json
import jsonschema
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


def convert_to_heimdall_format(services_data: Dict[str, Any]) -> Dict[str, Any]:
    
    heimdall_config = {
        "version": "1.0",
        "name": "My Services Dashboard",
        "description": "オンプレミスサービス一覧",
        "services": []
    }
    
    for service in services_data.get('services', []):
        healthcheck_url = None
        if service.get('healthcheck', False):
            base_url = service['url']
            if base_url.endswith('/'):
                healthcheck_url = f"{base_url}health"
            else:
                healthcheck_url = f"{base_url}/health"

        heimdall_service = {
            "name": service['name'],
            "description": service['description'],
            "url": service['url'],
            "host": service['host'],
            "category": service['category'],
            "labels": service.get('label', []),
            "aliases": service.get('alias-name', []),
            "external": service.get('external', False),
            "healthcheck": {
                "enabled": service.get('healthcheck', False),
                "url": healthcheck_url
            }
        }

        if service.get('image'):
            heimdall_service['icon'] = service['image']

        heimdall_config['services'].append(heimdall_service)
    
    return heimdall_config


def generate_heimdall_yaml(heimdall_config: Dict[str, Any], output_path: str) -> None:
    try:
        with open(output_path, 'w', encoding='utf-8') as file:
            yaml.dump(heimdall_config, file, default_flow_style=False, allow_unicode=True, sort_keys=False)
        print(f"Generated Heimdall config: {output_path}")
    except Exception as e:
        print(f"Failed to save file: {e}")
        sys.exit(1)


def main():
    parser = argparse.ArgumentParser(
        description='Convert services.yml to Heimdall dashboard configuration'
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
        default='heimdall_config.yaml',
        help='Output file path (default: heimdall_config.yaml)'
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
    
    print("Starting Heimdall configuration conversion...")
    print(f"   Input file: {args.services}")
    print(f"   Schema file: {args.schema}")
    print(f"   Output file: {args.output}")
    
    # ファイルの存在確認
    if not Path(args.services).exists():
        print(f"Error: File '{args.services}' does not exist.")
        sys.exit(1)
    
    if not Path(args.schema).exists():
        print(f"Error: File '{args.schema}' does not exist.")
        sys.exit(1)
    
    # YAMLファイルの読み込み
    print("\nLoading services.yml...")
    services_data = load_yaml_file(args.services)
    
    # JSON Schemaの読み込み
    print("Loading JSON Schema...")
    schema = load_json_schema(args.schema)
    
    # バリデーション（スキップしない場合）
    if not args.no_validate:
        print("\nValidating data...")
        if not validate_data(services_data, schema):
            print("Validation failed. Aborting.")
            sys.exit(1)
    else:
        print("Skipping validation.")

    # 検証のみの場合はここで終了
    if args.validate_only:
        print("\nValidation-only run. No conversion performed.")
        sys.exit(0)
    
    # Heimdall形式に変換
    print("\nConverting to Heimdall format...")
    heimdall_config = convert_to_heimdall_format(services_data)
    
    # 出力ファイルの生成
    print(f"\nWriting configuration file...")
    generate_heimdall_yaml(heimdall_config, args.output)
    
    print(f"\nConversion completed!")
    print(f"   Services converted: {len(heimdall_config['services'])}")
    print(f"   Output file: {args.output}")


if __name__ == "__main__":
    main()
