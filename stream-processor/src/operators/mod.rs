//! Stream processing operators.

use anyhow::Result;
use crate::engine::TelemetryData;

/// Trait for stream processing operators.
pub trait Operator {
    fn process(&mut self, data: &TelemetryData) -> Result<Option<TelemetryData>>;
}

/// Chain of operators for building pipelines.
pub struct OperatorChain {
    operators: Vec<Box<dyn Operator + Send + Sync>>,
}

impl OperatorChain {
    pub fn new() -> Self {
        Self {
            operators: Vec::new(),
        }
    }

    pub fn add<O: Operator + Send + Sync + 'static>(&mut self, op: O) {
        self.operators.push(Box::new(op));
    }

    pub fn process(&mut self, data: TelemetryData) -> Result<Option<TelemetryData>> {
        let mut current = Some(data);

        for op in &mut self.operators {
            if let Some(d) = current {
                current = op.process(&d)?;
            } else {
                break;
            }
        }

        Ok(current)
    }
}

/// Filter operator for dropping unwanted data.
pub struct FilterOperator {
    predicate: Box<dyn Fn(&TelemetryData) -> bool + Send + Sync>,
}

impl FilterOperator {
    pub fn new<F>(predicate: F) -> Self
    where
        F: Fn(&TelemetryData) -> bool + Send + Sync + 'static,
    {
        Self {
            predicate: Box::new(predicate),
        }
    }
}

impl Operator for FilterOperator {
    fn process(&mut self, data: &TelemetryData) -> Result<Option<TelemetryData>> {
        if (self.predicate)(data) {
            Ok(Some(data.clone()))
        } else {
            Ok(None)
        }
    }
}

/// Enrichment operator for adding metadata.
pub struct EnrichOperator {
    enrichments: std::collections::HashMap<String, String>,
}

impl EnrichOperator {
    pub fn new(enrichments: std::collections::HashMap<String, String>) -> Self {
        Self { enrichments }
    }
}

impl Operator for EnrichOperator {
    fn process(&mut self, data: &TelemetryData) -> Result<Option<TelemetryData>> {
        // Add enrichments to data
        // This is a simplified implementation
        Ok(Some(data.clone()))
    }
}

/// Sampling operator for reducing data volume.
pub struct SampleOperator {
    rate: f64,
    counter: u64,
}

impl SampleOperator {
    pub fn new(rate: f64) -> Self {
        Self { rate, counter: 0 }
    }
}

impl Operator for SampleOperator {
    fn process(&mut self, data: &TelemetryData) -> Result<Option<TelemetryData>> {
        self.counter += 1;
        let sample_threshold = (1.0 / self.rate) as u64;

        if self.counter % sample_threshold == 0 {
            Ok(Some(data.clone()))
        } else {
            Ok(None)
        }
    }
}

/// Aggregation operator for metrics.
pub struct AggregateOperator {
    window_seconds: u64,
    aggregations: std::collections::HashMap<String, AggregatedMetric>,
}

struct AggregatedMetric {
    sum: f64,
    count: u64,
    min: f64,
    max: f64,
}

impl AggregateOperator {
    pub fn new(window_seconds: u64) -> Self {
        Self {
            window_seconds,
            aggregations: std::collections::HashMap::new(),
        }
    }
}

impl Operator for AggregateOperator {
    fn process(&mut self, data: &TelemetryData) -> Result<Option<TelemetryData>> {
        if let TelemetryData::Metrics(metrics) = data {
            for metric in metrics {
                let agg = self.aggregations
                    .entry(metric.name.clone())
                    .or_insert_with(|| AggregatedMetric {
                        sum: 0.0,
                        count: 0,
                        min: f64::MAX,
                        max: f64::MIN,
                    });

                agg.sum += metric.value;
                agg.count += 1;
                agg.min = agg.min.min(metric.value);
                agg.max = agg.max.max(metric.value);
            }
        }

        Ok(Some(data.clone()))
    }
}
