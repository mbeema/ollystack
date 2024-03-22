package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mbeema/ollystack/api-server/internal/services"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

// QueryMetrics handles metrics queries.
func QueryMetrics(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := c.Query("query")
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
			return
		}

		result, err := svc.Metrics.Query(c.Request.Context(), query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// QueryMetricsRange handles range queries for metrics.
func QueryMetricsRange(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := c.Query("query")
		start := c.Query("start")
		end := c.Query("end")
		step := c.Query("step")

		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
			return
		}

		result, err := svc.Metrics.QueryRange(c.Request.Context(), query, start, end, step)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// ListLabels returns all label names.
func ListLabels(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		labels, err := svc.Metrics.ListLabels(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"labels": labels})
	}
}

// ListLabelValues returns values for a label.
func ListLabelValues(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		values, err := svc.Metrics.ListLabelValues(c.Request.Context(), name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"values": values})
	}
}

// ListSeries returns matching series.
func ListSeries(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		match := c.QueryArray("match[]")
		series, err := svc.Metrics.ListSeries(c.Request.Context(), match)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"series": series})
	}
}

// QueryLogs handles log queries.
func QueryLogs(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := c.Query("query")
		limit := c.DefaultQuery("limit", "100")
		direction := c.DefaultQuery("direction", "backward")

		result, err := svc.Logs.Query(c.Request.Context(), query, limit, direction)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// TailLogs streams logs via WebSocket.
func TailLogs(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := c.Query("query")

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Stream logs
		svc.Logs.Tail(c.Request.Context(), query, func(log interface{}) {
			conn.WriteJSON(log)
		})
	}
}

// ListLogLabels returns all log labels.
func ListLogLabels(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		labels, err := svc.Logs.ListLabels(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"labels": labels})
	}
}

// ListServices returns all services in the topology.
func ListServices(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		services, err := svc.Topology.ListServices(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"services": services})
	}
}

// ListEdges returns all edges in the topology.
func ListEdges(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		edges, err := svc.Topology.ListEdges(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"edges": edges})
	}
}

// GetTopologyGraph returns the complete topology graph.
func GetTopologyGraph(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		graph, err := svc.Topology.GetGraph(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, graph)
	}
}

// ListAlerts returns all alerts.
func ListAlerts(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		alerts, err := svc.Alerts.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"alerts": alerts})
	}
}

// CreateAlert creates a new alert.
func CreateAlert(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var alert services.Alert
		if err := c.ShouldBindJSON(&alert); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		created, err := svc.Alerts.Create(c.Request.Context(), &alert)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, created)
	}
}

// GetAlert returns an alert by ID.
func GetAlert(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		alert, err := svc.Alerts.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "alert not found"})
			return
		}
		c.JSON(http.StatusOK, alert)
	}
}

// UpdateAlert updates an alert.
func UpdateAlert(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var alert services.Alert
		if err := c.ShouldBindJSON(&alert); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		updated, err := svc.Alerts.Update(c.Request.Context(), id, &alert)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, updated)
	}
}

// DeleteAlert deletes an alert.
func DeleteAlert(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := svc.Alerts.Delete(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusNoContent, nil)
	}
}

// ListDashboards returns all dashboards.
func ListDashboards(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		dashboards, err := svc.Dashboards.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"dashboards": dashboards})
	}
}

// CreateDashboard creates a new dashboard.
func CreateDashboard(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var dashboard services.Dashboard
		if err := c.ShouldBindJSON(&dashboard); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		created, err := svc.Dashboards.Create(c.Request.Context(), &dashboard)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, created)
	}
}

// GetDashboard returns a dashboard by ID.
func GetDashboard(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		dashboard, err := svc.Dashboards.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "dashboard not found"})
			return
		}
		c.JSON(http.StatusOK, dashboard)
	}
}

// UpdateDashboard updates a dashboard.
func UpdateDashboard(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		var dashboard services.Dashboard
		if err := c.ShouldBindJSON(&dashboard); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		updated, err := svc.Dashboards.Update(c.Request.Context(), id, &dashboard)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, updated)
	}
}

// DeleteDashboard deletes a dashboard.
func DeleteDashboard(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := svc.Dashboards.Delete(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusNoContent, nil)
	}
}

// ExecuteQuery executes an ObservQL query.
func ExecuteQuery(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Query string `json:"query" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := svc.Query.Execute(c.Request.Context(), req.Query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// AskAI handles natural language queries.
func AskAI(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Question string `json:"question" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		response, err := svc.AI.Ask(c.Request.Context(), req.Question)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, response)
	}
}

// AnalyzeAnomaly analyzes an anomaly.
func AnalyzeAnomaly(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			TraceID string `json:"traceId"`
			Metric  string `json:"metric"`
			Start   int64  `json:"start"`
			End     int64  `json:"end"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		analysis, err := svc.AI.AnalyzeAnomaly(c.Request.Context(), req.TraceID, req.Metric)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, analysis)
	}
}

// GetSuggestions returns AI-generated suggestions.
func GetSuggestions(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		context := c.Query("context")
		suggestions, err := svc.AI.GetSuggestions(c.Request.Context(), context)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
	}
}

// WebSocket handles WebSocket connections for real-time updates.
func WebSocket(svc *services.Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Handle WebSocket messages
		for {
			var msg struct {
				Type    string `json:"type"`
				Channel string `json:"channel"`
			}
			if err := conn.ReadJSON(&msg); err != nil {
				break
			}

			switch msg.Type {
			case "subscribe":
				svc.PubSub.Subscribe(msg.Channel, func(data interface{}) {
					conn.WriteJSON(data)
				})
			case "unsubscribe":
				svc.PubSub.Unsubscribe(msg.Channel)
			}
		}
	}
}
