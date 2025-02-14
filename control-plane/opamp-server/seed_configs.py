#!/usr/bin/env python3
"""
Seed default configurations into the OpAMP Control Plane.
Run this after the server starts to populate initial configs.
"""

import os
import json
import hashlib
import asyncio
from datetime import datetime
from pathlib import Path

import redis.asyncio as redis

REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379")
CONFIGS_DIR = Path(__file__).parent.parent / "configs"


def generate_id():
    import uuid
    return str(uuid.uuid4())


def hash_config(config_yaml: str) -> str:
    return hashlib.sha256(config_yaml.encode()).hexdigest()[:16]


async def seed_configs():
    """Load config files and seed them into Redis."""
    r = await redis.from_url(REDIS_URL, decode_responses=True)

    configs = [
        {
            "file": "gateway-collector.yaml",
            "name": "gateway-collector",
            "description": "Gateway collector with host metrics, Docker stats, and log collection",
            "labels": {"type": "gateway", "tier": "infrastructure"},
            "status": "active",
        },
        {
            "file": "basic-otlp.yaml",
            "name": "basic-otlp",
            "description": "Minimal OTLP collector for simple forwarding",
            "labels": {"type": "sidecar", "complexity": "basic"},
            "status": "active",
        },
        {
            "file": "advanced-processors-demo.yaml",
            "name": "advanced-processors-demo",
            "description": "Demonstrates complex OTel processors: transform, filter, routing, sampling, grouping",
            "labels": {"type": "demo", "complexity": "advanced"},
            "status": "draft",
        },
    ]

    seeded = 0
    for config_info in configs:
        config_path = CONFIGS_DIR / config_info["file"]
        if not config_path.exists():
            print(f"Config file not found: {config_path}")
            continue

        # Check if config already exists by name
        existing_ids = await r.smembers("configs")
        exists = False
        for config_id in existing_ids:
            data = await r.hget(f"config:{config_id}", "data")
            if data:
                existing = json.loads(data)
                if existing.get("name") == config_info["name"]:
                    print(f"Config '{config_info['name']}' already exists, skipping")
                    exists = True
                    break

        if exists:
            continue

        # Read config file
        config_yaml = config_path.read_text()
        config_id = generate_id()
        now = datetime.utcnow().isoformat()

        config = {
            "id": config_id,
            "name": config_info["name"],
            "description": config_info["description"],
            "config_yaml": config_yaml,
            "config_hash": hash_config(config_yaml),
            "version": 1,
            "status": config_info["status"],
            "labels": config_info["labels"],
            "created_at": now,
            "updated_at": now,
        }

        await r.hset(f"config:{config_id}", mapping={"data": json.dumps(config)})
        await r.sadd("configs", config_id)
        print(f"Seeded config: {config_info['name']} ({config_id})")
        seeded += 1

    # Seed default environment
    env_ids = await r.smembers("environments")
    if not env_ids:
        env_id = generate_id()
        now = datetime.utcnow().isoformat()
        env = {
            "id": env_id,
            "name": "production",
            "description": "Production environment",
            "variables": {
                "OTEL_EXPORTER_ENDPOINT": "collector:4317",
                "CLICKHOUSE_HOST": "clickhouse",
                "CLICKHOUSE_PORT": "9000",
            },
            "created_at": now,
            "updated_at": now,
        }
        await r.hset(f"env:{env_id}", mapping={"data": json.dumps(env)})
        await r.sadd("environments", env_id)
        print(f"Seeded environment: production ({env_id})")
        seeded += 1

    # Seed default groups
    group_ids = await r.smembers("groups")
    if not group_ids:
        groups = [
            {"name": "gateway", "description": "Gateway collectors", "labels": {"tier": "gateway"}},
            {"name": "microservices", "description": "Application microservices", "labels": {"tier": "application"}},
            {"name": "infrastructure", "description": "Infrastructure services", "labels": {"tier": "infrastructure"}},
        ]

        # Get env ID for linking
        env_ids = await r.smembers("environments")
        env_id = list(env_ids)[0] if env_ids else None

        for group_info in groups:
            group_id = generate_id()
            now = datetime.utcnow().isoformat()
            group = {
                "id": group_id,
                "name": group_info["name"],
                "description": group_info["description"],
                "environment_id": env_id,
                "config_id": None,
                "labels": group_info["labels"],
                "agent_count": 0,
                "created_at": now,
                "updated_at": now,
            }
            await r.hset(f"group:{group_id}", mapping={"data": json.dumps(group)})
            await r.sadd("groups", group_id)
            print(f"Seeded group: {group_info['name']} ({group_id})")
            seeded += 1

    await r.close()
    print(f"\nSeeding complete. {seeded} items seeded.")


if __name__ == "__main__":
    asyncio.run(seed_configs())
