"""
OllyStack AI Engine - Main Entry Point

FastAPI server providing AI-powered observability features:
- Natural Language Query (NLQ) to ObservQL translation
- Anomaly detection and scoring
- Root cause analysis
- Predictive analytics
"""

import logging
import os
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from prometheus_client import make_asgi_app

from ollystack_ai.api import nlq, anomaly, rca, health, investigations
from ollystack_ai.services.llm import LLMService
from ollystack_ai.services.storage import StorageService
from ollystack_ai.services.cache import CacheService
from ollystack_ai.investigations.engine import InvestigationEngine
from ollystack_ai.investigations.triggers import InvestigationTrigger

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan manager."""
    logger.info("Starting OllyStack AI Engine...")

    # Initialize services
    app.state.llm_service = LLMService()
    app.state.storage_service = StorageService()
    app.state.cache_service = CacheService()

    await app.state.storage_service.connect()
    await app.state.cache_service.connect()

    # Initialize investigation engine and triggers
    app.state.investigation_engine = InvestigationEngine(
        llm_service=app.state.llm_service,
        storage_service=app.state.storage_service,
        cache_service=app.state.cache_service,
    )
    app.state.investigation_trigger = InvestigationTrigger(
        engine=app.state.investigation_engine,
        storage=app.state.storage_service,
        cache=app.state.cache_service,
    )

    # Start investigation trigger monitoring if enabled
    if os.getenv("ENABLE_INVESTIGATION_TRIGGERS", "true").lower() == "true":
        await app.state.investigation_trigger.start()
        logger.info("Investigation trigger monitoring started")

    logger.info("AI Engine initialized successfully")
    yield

    # Cleanup
    logger.info("Shutting down AI Engine...")

    # Stop investigation triggers
    if hasattr(app.state, 'investigation_trigger'):
        await app.state.investigation_trigger.stop()

    await app.state.storage_service.disconnect()
    await app.state.cache_service.disconnect()


def create_app() -> FastAPI:
    """Create and configure the FastAPI application."""
    app = FastAPI(
        title="OllyStack AI Engine",
        description="AI-powered observability: NLQ, Anomaly Detection, Root Cause Analysis",
        version="0.1.0",
        lifespan=lifespan,
    )

    # CORS middleware
    app.add_middleware(
        CORSMiddleware,
        allow_origins=os.getenv("CORS_ORIGINS", "*").split(","),
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # Mount Prometheus metrics
    metrics_app = make_asgi_app()
    app.mount("/metrics", metrics_app)

    # Include routers
    app.include_router(health.router, tags=["Health"])
    app.include_router(nlq.router, prefix="/api/v1/nlq", tags=["Natural Language Query"])
    app.include_router(anomaly.router, prefix="/api/v1/anomaly", tags=["Anomaly Detection"])
    app.include_router(rca.router, prefix="/api/v1/rca", tags=["Root Cause Analysis"])
    app.include_router(investigations.router, prefix="/api/v1/investigations", tags=["Proactive Investigations"])

    return app


app = create_app()


def main():
    """Run the server."""
    import uvicorn

    uvicorn.run(
        "ollystack_ai.main:app",
        host=os.getenv("HOST", "0.0.0.0"),
        port=int(os.getenv("PORT", "8081")),
        reload=os.getenv("RELOAD", "false").lower() == "true",
        workers=int(os.getenv("WORKERS", "1")),
    )


if __name__ == "__main__":
    main()
