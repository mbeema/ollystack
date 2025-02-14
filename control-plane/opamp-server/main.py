"""
OllyStack OpAMP Control Plane
Central configuration management for OTel Collectors using OpAMP-like protocol.

Features:
- Agent enrollment and management
- Configuration management (create, version, deploy)
- Groups and environments for organizing agents
- WebSocket for real-time agent communication
- Push configuration updates to agents
- REST API for React UI integration
"""

import os
import json
import uuid
import asyncio
import hashlib
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Set
from dataclasses import dataclass, asdict, field
from enum import Enum

from fastapi import FastAPI, HTTPException, WebSocket, WebSocketDisconnect, Query, Depends
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
import redis.asyncio as redis
import yaml

# =============================================================================
# Configuration
# =============================================================================

REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379")
HOST = os.getenv("HOST", "0.0.0.0")
PORT = int(os.getenv("PORT", "4320"))

# =============================================================================
# Data Models
# =============================================================================

class AgentStatus(str, Enum):
    PENDING = "pending"        # Registered but not connected
    CONNECTED = "connected"    # WebSocket connected
    HEALTHY = "healthy"        # Connected and reporting healthy
    UNHEALTHY = "unhealthy"    # Connected but reporting errors
    DISCONNECTED = "disconnected"  # Was connected, now disconnected


class ConfigStatus(str, Enum):
    DRAFT = "draft"
    ACTIVE = "active"
    ARCHIVED = "archived"


# Pydantic models for API
class EnvironmentCreate(BaseModel):
    name: str = Field(..., min_length=1, max_length=100)
    description: Optional[str] = None
    variables: Dict[str, str] = Field(default_factory=dict)


class EnvironmentUpdate(BaseModel):
    name: Optional[str] = None
    description: Optional[str] = None
    variables: Optional[Dict[str, str]] = None


class Environment(BaseModel):
    id: str
    name: str
    description: Optional[str] = None
    variables: Dict[str, str] = Field(default_factory=dict)
    created_at: datetime
    updated_at: datetime


class GroupCreate(BaseModel):
    name: str = Field(..., min_length=1, max_length=100)
    description: Optional[str] = None
    environment_id: Optional[str] = None
    labels: Dict[str, str] = Field(default_factory=dict)


class GroupUpdate(BaseModel):
    name: Optional[str] = None
    description: Optional[str] = None
    environment_id: Optional[str] = None
    labels: Optional[Dict[str, str]] = None
    config_id: Optional[str] = None


class Group(BaseModel):
    id: str
    name: str
    description: Optional[str] = None
    environment_id: Optional[str] = None
    config_id: Optional[str] = None
    labels: Dict[str, str] = Field(default_factory=dict)
    agent_count: int = 0
    created_at: datetime
    updated_at: datetime


class ConfigCreate(BaseModel):
    name: str = Field(..., min_length=1, max_length=100)
    description: Optional[str] = None
    config_yaml: str  # The actual YAML configuration
    labels: Dict[str, str] = Field(default_factory=dict)


class ConfigUpdate(BaseModel):
    name: Optional[str] = None
    description: Optional[str] = None
    config_yaml: Optional[str] = None
    labels: Optional[Dict[str, str]] = None
    status: Optional[ConfigStatus] = None


class Config(BaseModel):
    id: str
    name: str
    description: Optional[str] = None
    config_yaml: str
    config_hash: str
    version: int
    status: ConfigStatus
    labels: Dict[str, str] = Field(default_factory=dict)
    created_at: datetime
    updated_at: datetime


class AgentRegister(BaseModel):
    agent_id: Optional[str] = None  # If not provided, will be generated
    hostname: str
    ip_address: Optional[str] = None
    group_id: Optional[str] = None
    labels: Dict[str, str] = Field(default_factory=dict)
    capabilities: List[str] = Field(default_factory=list)


class AgentUpdate(BaseModel):
    group_id: Optional[str] = None
    labels: Optional[Dict[str, str]] = None


class Agent(BaseModel):
    id: str
    hostname: str
    ip_address: Optional[str] = None
    group_id: Optional[str] = None
    status: AgentStatus
    labels: Dict[str, str] = Field(default_factory=dict)
    capabilities: List[str] = Field(default_factory=list)
    effective_config_hash: Optional[str] = None
    last_seen: Optional[datetime] = None
    last_error: Optional[str] = None
    created_at: datetime
    updated_at: datetime


class ConfigPush(BaseModel):
    config_id: str
    target_type: str = "group"  # "group", "agent", "environment"
    target_id: str


class AgentMessage(BaseModel):
    """Message from agent to server"""
    type: str  # "status", "config_request", "heartbeat", "error"
    agent_id: str
    payload: Dict = Field(default_factory=dict)


class ServerMessage(BaseModel):
    """Message from server to agent"""
    type: str  # "config_update", "restart", "ping", "ack"
    payload: Dict = Field(default_factory=dict)


# =============================================================================
# Application State
# =============================================================================

class ControlPlaneState:
    def __init__(self):
        self.redis: Optional[redis.Redis] = None
        self.connected_agents: Dict[str, WebSocket] = {}  # agent_id -> websocket
        self.agent_tasks: Dict[str, asyncio.Task] = {}

    async def init_redis(self):
        self.redis = redis.from_url(REDIS_URL, decode_responses=True)
        await self.redis.ping()
        print(f"Connected to Redis at {REDIS_URL}")

    async def close(self):
        if self.redis:
            await self.redis.close()


state = ControlPlaneState()

# =============================================================================
# FastAPI Application
# =============================================================================

app = FastAPI(
    title="OllyStack Control Plane",
    description="Central configuration management for OTel Collectors",
    version="1.0.0",
    docs_url="/docs",
    redoc_url="/redoc"
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.on_event("startup")
async def startup():
    await state.init_redis()


@app.on_event("shutdown")
async def shutdown():
    await state.close()


# =============================================================================
# Helper Functions
# =============================================================================

def generate_id() -> str:
    return str(uuid.uuid4())


def hash_config(config_yaml: str) -> str:
    return hashlib.sha256(config_yaml.encode()).hexdigest()[:16]


def now() -> datetime:
    return datetime.utcnow()


async def get_redis() -> redis.Redis:
    if not state.redis:
        raise HTTPException(status_code=503, detail="Redis not connected")
    return state.redis


# =============================================================================
# Environment Endpoints
# =============================================================================

@app.post("/api/v1/environments", response_model=Environment, tags=["Environments"])
async def create_environment(env: EnvironmentCreate, r: redis.Redis = Depends(get_redis)):
    """Create a new environment"""
    env_id = generate_id()
    now_time = now()

    environment = Environment(
        id=env_id,
        name=env.name,
        description=env.description,
        variables=env.variables,
        created_at=now_time,
        updated_at=now_time
    )

    await r.hset(f"env:{env_id}", mapping={
        "data": environment.model_dump_json()
    })
    await r.sadd("environments", env_id)

    return environment


@app.get("/api/v1/environments", response_model=List[Environment], tags=["Environments"])
async def list_environments(r: redis.Redis = Depends(get_redis)):
    """List all environments"""
    env_ids = await r.smembers("environments")
    environments = []

    for env_id in env_ids:
        data = await r.hget(f"env:{env_id}", "data")
        if data:
            environments.append(Environment.model_validate_json(data))

    return sorted(environments, key=lambda x: x.name)


@app.get("/api/v1/environments/{env_id}", response_model=Environment, tags=["Environments"])
async def get_environment(env_id: str, r: redis.Redis = Depends(get_redis)):
    """Get environment by ID"""
    data = await r.hget(f"env:{env_id}", "data")
    if not data:
        raise HTTPException(status_code=404, detail="Environment not found")
    return Environment.model_validate_json(data)


@app.put("/api/v1/environments/{env_id}", response_model=Environment, tags=["Environments"])
async def update_environment(env_id: str, update: EnvironmentUpdate, r: redis.Redis = Depends(get_redis)):
    """Update an environment"""
    data = await r.hget(f"env:{env_id}", "data")
    if not data:
        raise HTTPException(status_code=404, detail="Environment not found")

    env = Environment.model_validate_json(data)

    if update.name is not None:
        env.name = update.name
    if update.description is not None:
        env.description = update.description
    if update.variables is not None:
        env.variables = update.variables
    env.updated_at = now()

    await r.hset(f"env:{env_id}", mapping={"data": env.model_dump_json()})
    return env


@app.delete("/api/v1/environments/{env_id}", tags=["Environments"])
async def delete_environment(env_id: str, r: redis.Redis = Depends(get_redis)):
    """Delete an environment"""
    await r.delete(f"env:{env_id}")
    await r.srem("environments", env_id)
    return {"status": "deleted"}


# =============================================================================
# Group Endpoints
# =============================================================================

@app.post("/api/v1/groups", response_model=Group, tags=["Groups"])
async def create_group(group: GroupCreate, r: redis.Redis = Depends(get_redis)):
    """Create a new group"""
    group_id = generate_id()
    now_time = now()

    new_group = Group(
        id=group_id,
        name=group.name,
        description=group.description,
        environment_id=group.environment_id,
        labels=group.labels,
        created_at=now_time,
        updated_at=now_time
    )

    await r.hset(f"group:{group_id}", mapping={"data": new_group.model_dump_json()})
    await r.sadd("groups", group_id)

    return new_group


@app.get("/api/v1/groups", response_model=List[Group], tags=["Groups"])
async def list_groups(
    environment_id: Optional[str] = None,
    r: redis.Redis = Depends(get_redis)
):
    """List all groups, optionally filtered by environment"""
    group_ids = await r.smembers("groups")
    groups = []

    for group_id in group_ids:
        data = await r.hget(f"group:{group_id}", "data")
        if data:
            group = Group.model_validate_json(data)

            # Count agents in this group
            agent_ids = await r.smembers("agents")
            count = 0
            for agent_id in agent_ids:
                agent_data = await r.hget(f"agent:{agent_id}", "data")
                if agent_data:
                    agent = Agent.model_validate_json(agent_data)
                    if agent.group_id == group_id:
                        count += 1
            group.agent_count = count

            if environment_id is None or group.environment_id == environment_id:
                groups.append(group)

    return sorted(groups, key=lambda x: x.name)


@app.get("/api/v1/groups/{group_id}", response_model=Group, tags=["Groups"])
async def get_group(group_id: str, r: redis.Redis = Depends(get_redis)):
    """Get group by ID"""
    data = await r.hget(f"group:{group_id}", "data")
    if not data:
        raise HTTPException(status_code=404, detail="Group not found")
    return Group.model_validate_json(data)


@app.put("/api/v1/groups/{group_id}", response_model=Group, tags=["Groups"])
async def update_group(group_id: str, update: GroupUpdate, r: redis.Redis = Depends(get_redis)):
    """Update a group"""
    data = await r.hget(f"group:{group_id}", "data")
    if not data:
        raise HTTPException(status_code=404, detail="Group not found")

    group = Group.model_validate_json(data)

    if update.name is not None:
        group.name = update.name
    if update.description is not None:
        group.description = update.description
    if update.environment_id is not None:
        group.environment_id = update.environment_id
    if update.labels is not None:
        group.labels = update.labels
    if update.config_id is not None:
        group.config_id = update.config_id
    group.updated_at = now()

    await r.hset(f"group:{group_id}", mapping={"data": group.model_dump_json()})

    # If config was updated, push to all agents in group
    if update.config_id is not None:
        await push_config_to_group(group_id, update.config_id, r)

    return group


@app.delete("/api/v1/groups/{group_id}", tags=["Groups"])
async def delete_group(group_id: str, r: redis.Redis = Depends(get_redis)):
    """Delete a group"""
    await r.delete(f"group:{group_id}")
    await r.srem("groups", group_id)
    return {"status": "deleted"}


# =============================================================================
# Configuration Endpoints
# =============================================================================

@app.post("/api/v1/configs", response_model=Config, tags=["Configurations"])
async def create_config(config: ConfigCreate, r: redis.Redis = Depends(get_redis)):
    """Create a new configuration"""
    # Validate YAML
    try:
        yaml.safe_load(config.config_yaml)
    except yaml.YAMLError as e:
        raise HTTPException(status_code=400, detail=f"Invalid YAML: {str(e)}")

    config_id = generate_id()
    now_time = now()
    config_hash = hash_config(config.config_yaml)

    new_config = Config(
        id=config_id,
        name=config.name,
        description=config.description,
        config_yaml=config.config_yaml,
        config_hash=config_hash,
        version=1,
        status=ConfigStatus.DRAFT,
        labels=config.labels,
        created_at=now_time,
        updated_at=now_time
    )

    await r.hset(f"config:{config_id}", mapping={"data": new_config.model_dump_json()})
    await r.sadd("configs", config_id)

    return new_config


@app.get("/api/v1/configs", response_model=List[Config], tags=["Configurations"])
async def list_configs(
    status: Optional[ConfigStatus] = None,
    r: redis.Redis = Depends(get_redis)
):
    """List all configurations"""
    config_ids = await r.smembers("configs")
    configs = []

    for config_id in config_ids:
        data = await r.hget(f"config:{config_id}", "data")
        if data:
            config = Config.model_validate_json(data)
            if status is None or config.status == status:
                configs.append(config)

    return sorted(configs, key=lambda x: x.updated_at, reverse=True)


@app.get("/api/v1/configs/{config_id}", response_model=Config, tags=["Configurations"])
async def get_config(config_id: str, r: redis.Redis = Depends(get_redis)):
    """Get configuration by ID"""
    data = await r.hget(f"config:{config_id}", "data")
    if not data:
        raise HTTPException(status_code=404, detail="Configuration not found")
    return Config.model_validate_json(data)


@app.put("/api/v1/configs/{config_id}", response_model=Config, tags=["Configurations"])
async def update_config(config_id: str, update: ConfigUpdate, r: redis.Redis = Depends(get_redis)):
    """Update a configuration (creates new version if YAML changes)"""
    data = await r.hget(f"config:{config_id}", "data")
    if not data:
        raise HTTPException(status_code=404, detail="Configuration not found")

    config = Config.model_validate_json(data)

    if update.name is not None:
        config.name = update.name
    if update.description is not None:
        config.description = update.description
    if update.labels is not None:
        config.labels = update.labels
    if update.status is not None:
        config.status = update.status

    if update.config_yaml is not None:
        # Validate YAML
        try:
            yaml.safe_load(update.config_yaml)
        except yaml.YAMLError as e:
            raise HTTPException(status_code=400, detail=f"Invalid YAML: {str(e)}")

        config.config_yaml = update.config_yaml
        config.config_hash = hash_config(update.config_yaml)
        config.version += 1

    config.updated_at = now()

    await r.hset(f"config:{config_id}", mapping={"data": config.model_dump_json()})

    return config


@app.delete("/api/v1/configs/{config_id}", tags=["Configurations"])
async def delete_config(config_id: str, r: redis.Redis = Depends(get_redis)):
    """Delete a configuration"""
    await r.delete(f"config:{config_id}")
    await r.srem("configs", config_id)
    return {"status": "deleted"}


@app.post("/api/v1/configs/{config_id}/activate", response_model=Config, tags=["Configurations"])
async def activate_config(config_id: str, r: redis.Redis = Depends(get_redis)):
    """Activate a configuration (mark as ready for deployment)"""
    data = await r.hget(f"config:{config_id}", "data")
    if not data:
        raise HTTPException(status_code=404, detail="Configuration not found")

    config = Config.model_validate_json(data)
    config.status = ConfigStatus.ACTIVE
    config.updated_at = now()

    await r.hset(f"config:{config_id}", mapping={"data": config.model_dump_json()})
    return config


# =============================================================================
# Agent Endpoints
# =============================================================================

@app.post("/api/v1/agents/register", response_model=Agent, tags=["Agents"])
async def register_agent(agent: AgentRegister, r: redis.Redis = Depends(get_redis)):
    """Register a new agent or update existing"""
    agent_id = agent.agent_id or generate_id()
    now_time = now()

    # Check if agent already exists
    existing = await r.hget(f"agent:{agent_id}", "data")
    if existing:
        existing_agent = Agent.model_validate_json(existing)
        existing_agent.hostname = agent.hostname
        existing_agent.ip_address = agent.ip_address
        if agent.group_id:
            existing_agent.group_id = agent.group_id
        existing_agent.labels.update(agent.labels)
        existing_agent.capabilities = agent.capabilities
        existing_agent.status = AgentStatus.PENDING
        existing_agent.updated_at = now_time

        await r.hset(f"agent:{agent_id}", mapping={"data": existing_agent.model_dump_json()})
        return existing_agent

    new_agent = Agent(
        id=agent_id,
        hostname=agent.hostname,
        ip_address=agent.ip_address,
        group_id=agent.group_id,
        status=AgentStatus.PENDING,
        labels=agent.labels,
        capabilities=agent.capabilities,
        created_at=now_time,
        updated_at=now_time
    )

    await r.hset(f"agent:{agent_id}", mapping={"data": new_agent.model_dump_json()})
    await r.sadd("agents", agent_id)

    return new_agent


@app.get("/api/v1/agents", response_model=List[Agent], tags=["Agents"])
async def list_agents(
    group_id: Optional[str] = None,
    status: Optional[AgentStatus] = None,
    r: redis.Redis = Depends(get_redis)
):
    """List all agents"""
    agent_ids = await r.smembers("agents")
    agents = []

    for agent_id in agent_ids:
        data = await r.hget(f"agent:{agent_id}", "data")
        if data:
            agent = Agent.model_validate_json(data)

            # Check if connected
            if agent_id in state.connected_agents:
                if agent.status == AgentStatus.PENDING:
                    agent.status = AgentStatus.CONNECTED

            if group_id is not None and agent.group_id != group_id:
                continue
            if status is not None and agent.status != status:
                continue

            agents.append(agent)

    return sorted(agents, key=lambda x: x.hostname)


@app.get("/api/v1/agents/{agent_id}", response_model=Agent, tags=["Agents"])
async def get_agent(agent_id: str, r: redis.Redis = Depends(get_redis)):
    """Get agent by ID"""
    data = await r.hget(f"agent:{agent_id}", "data")
    if not data:
        raise HTTPException(status_code=404, detail="Agent not found")
    return Agent.model_validate_json(data)


@app.put("/api/v1/agents/{agent_id}", response_model=Agent, tags=["Agents"])
async def update_agent(agent_id: str, update: AgentUpdate, r: redis.Redis = Depends(get_redis)):
    """Update agent"""
    data = await r.hget(f"agent:{agent_id}", "data")
    if not data:
        raise HTTPException(status_code=404, detail="Agent not found")

    agent = Agent.model_validate_json(data)

    if update.group_id is not None:
        agent.group_id = update.group_id
    if update.labels is not None:
        agent.labels = update.labels
    agent.updated_at = now()

    await r.hset(f"agent:{agent_id}", mapping={"data": agent.model_dump_json()})

    # If group changed, push new config
    if update.group_id is not None:
        await push_config_to_agent(agent_id, r)

    return agent


@app.delete("/api/v1/agents/{agent_id}", tags=["Agents"])
async def delete_agent(agent_id: str, r: redis.Redis = Depends(get_redis)):
    """Delete an agent"""
    # Disconnect if connected
    if agent_id in state.connected_agents:
        ws = state.connected_agents[agent_id]
        await ws.close()
        del state.connected_agents[agent_id]

    await r.delete(f"agent:{agent_id}")
    await r.srem("agents", agent_id)
    return {"status": "deleted"}


# =============================================================================
# Config Push Endpoints
# =============================================================================

@app.post("/api/v1/configs/push", tags=["Configurations"])
async def push_config(push: ConfigPush, r: redis.Redis = Depends(get_redis)):
    """Push a configuration to agents"""
    # Verify config exists
    config_data = await r.hget(f"config:{push.config_id}", "data")
    if not config_data:
        raise HTTPException(status_code=404, detail="Configuration not found")

    config = Config.model_validate_json(config_data)
    pushed_agents = []

    if push.target_type == "agent":
        success = await push_config_to_agent(push.target_id, r, config)
        if success:
            pushed_agents.append(push.target_id)

    elif push.target_type == "group":
        # Update group's config
        group_data = await r.hget(f"group:{push.target_id}", "data")
        if group_data:
            group = Group.model_validate_json(group_data)
            group.config_id = push.config_id
            group.updated_at = now()
            await r.hset(f"group:{push.target_id}", mapping={"data": group.model_dump_json()})

        pushed_agents = await push_config_to_group(push.target_id, push.config_id, r)

    elif push.target_type == "environment":
        # Push to all groups in environment
        group_ids = await r.smembers("groups")
        for group_id in group_ids:
            group_data = await r.hget(f"group:{group_id}", "data")
            if group_data:
                group = Group.model_validate_json(group_data)
                if group.environment_id == push.target_id:
                    group.config_id = push.config_id
                    group.updated_at = now()
                    await r.hset(f"group:{group_id}", mapping={"data": group.model_dump_json()})
                    agents = await push_config_to_group(group_id, push.config_id, r)
                    pushed_agents.extend(agents)

    return {
        "status": "pushed",
        "config_id": push.config_id,
        "config_version": config.version,
        "pushed_agents": pushed_agents,
        "pushed_count": len(pushed_agents)
    }


async def push_config_to_group(group_id: str, config_id: str, r: redis.Redis) -> List[str]:
    """Push config to all agents in a group"""
    config_data = await r.hget(f"config:{config_id}", "data")
    if not config_data:
        return []

    config = Config.model_validate_json(config_data)
    pushed_agents = []

    agent_ids = await r.smembers("agents")
    for agent_id in agent_ids:
        agent_data = await r.hget(f"agent:{agent_id}", "data")
        if agent_data:
            agent = Agent.model_validate_json(agent_data)
            if agent.group_id == group_id:
                success = await push_config_to_agent(agent_id, r, config)
                if success:
                    pushed_agents.append(agent_id)

    return pushed_agents


async def push_config_to_agent(agent_id: str, r: redis.Redis, config: Optional[Config] = None) -> bool:
    """Push config to a specific agent"""
    if agent_id not in state.connected_agents:
        return False

    # Get config from agent's group if not provided
    if config is None:
        agent_data = await r.hget(f"agent:{agent_id}", "data")
        if not agent_data:
            return False
        agent = Agent.model_validate_json(agent_data)

        if not agent.group_id:
            return False

        group_data = await r.hget(f"group:{agent.group_id}", "data")
        if not group_data:
            return False
        group = Group.model_validate_json(group_data)

        if not group.config_id:
            return False

        config_data = await r.hget(f"config:{group.config_id}", "data")
        if not config_data:
            return False
        config = Config.model_validate_json(config_data)

    # Send config to agent via WebSocket
    ws = state.connected_agents[agent_id]
    try:
        message = ServerMessage(
            type="config_update",
            payload={
                "config_id": config.id,
                "config_hash": config.config_hash,
                "config_version": config.version,
                "config_yaml": config.config_yaml
            }
        )
        await ws.send_json(message.model_dump())

        # Update agent's effective config hash
        agent_data = await r.hget(f"agent:{agent_id}", "data")
        if agent_data:
            agent = Agent.model_validate_json(agent_data)
            agent.effective_config_hash = config.config_hash
            agent.updated_at = now()
            await r.hset(f"agent:{agent_id}", mapping={"data": agent.model_dump_json()})

        return True
    except Exception as e:
        print(f"Failed to push config to agent {agent_id}: {e}")
        return False


# =============================================================================
# WebSocket for Agent Communication
# =============================================================================

@app.websocket("/ws/agent/{agent_id}")
async def agent_websocket(websocket: WebSocket, agent_id: str):
    """WebSocket endpoint for agent communication"""
    await websocket.accept()

    r = await get_redis()

    # Verify agent is registered
    agent_data = await r.hget(f"agent:{agent_id}", "data")
    if not agent_data:
        await websocket.close(code=4001, reason="Agent not registered")
        return

    # Update agent status
    agent = Agent.model_validate_json(agent_data)
    agent.status = AgentStatus.CONNECTED
    agent.last_seen = now()
    await r.hset(f"agent:{agent_id}", mapping={"data": agent.model_dump_json()})

    # Store connection
    state.connected_agents[agent_id] = websocket
    print(f"Agent {agent_id} connected")

    try:
        # Send current config if agent has a group
        if agent.group_id:
            await push_config_to_agent(agent_id, r)

        # Handle messages
        while True:
            data = await websocket.receive_json()
            message = AgentMessage.model_validate(data)

            if message.type == "heartbeat":
                # Update last seen
                agent_data = await r.hget(f"agent:{agent_id}", "data")
                if agent_data:
                    agent = Agent.model_validate_json(agent_data)
                    agent.last_seen = now()
                    agent.status = AgentStatus.HEALTHY
                    await r.hset(f"agent:{agent_id}", mapping={"data": agent.model_dump_json()})

                # Send pong
                await websocket.send_json({"type": "pong", "payload": {}})

            elif message.type == "status":
                # Update agent status
                agent_data = await r.hget(f"agent:{agent_id}", "data")
                if agent_data:
                    agent = Agent.model_validate_json(agent_data)
                    agent.last_seen = now()
                    if message.payload.get("healthy", True):
                        agent.status = AgentStatus.HEALTHY
                    else:
                        agent.status = AgentStatus.UNHEALTHY
                        agent.last_error = message.payload.get("error")
                    agent.effective_config_hash = message.payload.get("config_hash")
                    await r.hset(f"agent:{agent_id}", mapping={"data": agent.model_dump_json()})

            elif message.type == "config_request":
                # Agent requesting its config
                await push_config_to_agent(agent_id, r)

            elif message.type == "config_applied":
                # Agent confirming config was applied
                agent_data = await r.hget(f"agent:{agent_id}", "data")
                if agent_data:
                    agent = Agent.model_validate_json(agent_data)
                    agent.effective_config_hash = message.payload.get("config_hash")
                    agent.last_seen = now()
                    await r.hset(f"agent:{agent_id}", mapping={"data": agent.model_dump_json()})

    except WebSocketDisconnect:
        print(f"Agent {agent_id} disconnected")
    except Exception as e:
        print(f"Agent {agent_id} error: {e}")
    finally:
        # Update agent status
        if agent_id in state.connected_agents:
            del state.connected_agents[agent_id]

        agent_data = await r.hget(f"agent:{agent_id}", "data")
        if agent_data:
            agent = Agent.model_validate_json(agent_data)
            agent.status = AgentStatus.DISCONNECTED
            agent.last_seen = now()
            await r.hset(f"agent:{agent_id}", mapping={"data": agent.model_dump_json()})


# =============================================================================
# Fleet Status Endpoints
# =============================================================================

@app.get("/api/v1/fleet/status", tags=["Fleet"])
async def get_fleet_status(r: redis.Redis = Depends(get_redis)):
    """Get overall fleet status"""
    agent_ids = await r.smembers("agents")

    total = len(agent_ids)
    healthy = 0
    unhealthy = 0
    connected = 0
    disconnected = 0
    pending = 0

    for agent_id in agent_ids:
        agent_data = await r.hget(f"agent:{agent_id}", "data")
        if agent_data:
            agent = Agent.model_validate_json(agent_data)

            # Override status if connected
            if agent_id in state.connected_agents:
                if agent.status in [AgentStatus.PENDING, AgentStatus.DISCONNECTED]:
                    agent.status = AgentStatus.CONNECTED

            if agent.status == AgentStatus.HEALTHY:
                healthy += 1
            elif agent.status == AgentStatus.UNHEALTHY:
                unhealthy += 1
            elif agent.status == AgentStatus.CONNECTED:
                connected += 1
            elif agent.status == AgentStatus.DISCONNECTED:
                disconnected += 1
            elif agent.status == AgentStatus.PENDING:
                pending += 1

    config_count = len(await r.smembers("configs"))
    group_count = len(await r.smembers("groups"))
    env_count = len(await r.smembers("environments"))

    return {
        "agents": {
            "total": total,
            "healthy": healthy,
            "unhealthy": unhealthy,
            "connected": connected,
            "disconnected": disconnected,
            "pending": pending
        },
        "configs": config_count,
        "groups": group_count,
        "environments": env_count,
        "timestamp": now().isoformat()
    }


@app.get("/api/v1/health", tags=["Health"])
async def health_check():
    """Health check endpoint"""
    redis_ok = False
    try:
        if state.redis:
            await state.redis.ping()
            redis_ok = True
    except:
        pass

    return {
        "status": "healthy" if redis_ok else "degraded",
        "redis": "connected" if redis_ok else "disconnected",
        "connected_agents": len(state.connected_agents),
        "timestamp": now().isoformat()
    }


# =============================================================================
# Default Config Templates
# =============================================================================

@app.get("/api/v1/templates", tags=["Templates"])
async def get_config_templates():
    """Get pre-defined configuration templates"""
    return {
        "templates": [
            {
                "id": "basic-otlp",
                "name": "Basic OTLP Collector",
                "description": "Simple collector with OTLP receivers and ClickHouse exporter",
                "config_yaml": get_basic_template()
            },
            {
                "id": "with-filelog",
                "name": "OTLP + File Log Collection",
                "description": "Collector with OTLP and Docker file log collection",
                "config_yaml": get_filelog_template()
            },
            {
                "id": "full-featured",
                "name": "Full Featured Collector",
                "description": "Complete collector with all processors and exporters",
                "config_yaml": get_full_template()
            }
        ]
    }


def get_basic_template() -> str:
    return """receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 5s
    send_batch_size: 10000
  memory_limiter:
    check_interval: 1s
    limit_mib: 512

exporters:
  clickhouse:
    endpoint: tcp://clickhouse:9000
    database: ollystack
    username: ollystack
    password: ${CLICKHOUSE_PASSWORD}
    logs_table_name: logs
    traces_table_name: traces
    metrics_table_name: metrics

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [clickhouse]
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [clickhouse]
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [clickhouse]
"""


def get_filelog_template() -> str:
    return """receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

  filelog/docker:
    include:
      - /var/lib/docker/containers/*/*.log
    start_at: end
    operators:
      - type: json_parser
        timestamp:
          parse_from: attributes.time
          layout: '%Y-%m-%dT%H:%M:%S.%LZ'
        on_error: drop

processors:
  batch:
    timeout: 5s
  memory_limiter:
    check_interval: 1s
    limit_mib: 512

exporters:
  clickhouse:
    endpoint: tcp://clickhouse:9000
    database: ollystack
    username: ollystack
    password: ${CLICKHOUSE_PASSWORD}

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [clickhouse]
    logs:
      receivers: [otlp, filelog/docker]
      processors: [memory_limiter, batch]
      exporters: [clickhouse]
"""


def get_full_template() -> str:
    return """receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

  filelog/docker:
    include:
      - /var/lib/docker/containers/*/*.log
    start_at: end
    operators:
      - type: json_parser
        timestamp:
          parse_from: attributes.time
          layout: '%Y-%m-%dT%H:%M:%S.%LZ'
        on_error: drop
      - type: router
        routes:
          - expr: 'attributes.log matches "^\\\\{.*\\\"service\\\".*\\\\}"'
            output: json_parser
        default: move_log
      - type: json_parser
        id: json_parser
        parse_from: attributes.log
        parse_to: body
        on_error: send
      - type: move
        id: move_log
        from: attributes.log
        to: body

processors:
  batch:
    timeout: 5s
    send_batch_size: 10000
  memory_limiter:
    check_interval: 1s
    limit_mib: 512
    spike_limit_mib: 128
  resource:
    attributes:
      - key: deployment.environment
        value: ${DEPLOYMENT_ENV:production}
        action: upsert

exporters:
  clickhouse:
    endpoint: tcp://clickhouse:9000
    database: ollystack
    username: ollystack
    password: ${CLICKHOUSE_PASSWORD}
    ttl: 168h
    logs_table_name: logs
    traces_table_name: traces
    metrics_table_name: metrics
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s

  debug:
    verbosity: basic

service:
  telemetry:
    logs:
      level: warn
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, resource, batch]
      exporters: [clickhouse]
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, resource, batch]
      exporters: [clickhouse]
    logs:
      receivers: [otlp, filelog/docker]
      processors: [memory_limiter, resource, batch]
      exporters: [clickhouse]
"""


# =============================================================================
# Main
# =============================================================================

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host=HOST, port=PORT)
