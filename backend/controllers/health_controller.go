package controllers

import (
	"net/http"
	"time"

	"mycorrhizal/buildinfo"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HealthResponse represents the health check response structure.
//
// Version/Commit/BuildDate come from the buildinfo package, injected at link
// time. They were previously a hardcoded "0.1.0" string literal, so every
// build ever made reported the same version and a bug report could not be
// tied to the binary that produced it.
type HealthResponse struct {
	Status    string         `json:"status"`
	Timestamp string         `json:"timestamp"`
	Database  DatabaseHealth `json:"database"`
	Version   string         `json:"version"`
	Commit    string         `json:"commit,omitempty"`
	BuildDate string         `json:"build_date,omitempty"`
}

// DatabaseHealth represents the database health status
type DatabaseHealth struct {
	Status       string  `json:"status"`
	ResponseTime float64 `json:"response_time_ms"`
}

// HealthCheck handles the health check endpoint
// GET /health
func HealthCheck(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	// Check database connectivity
	dbHealth := checkDatabaseHealth(db)

	// Determine overall status
	status := "healthy"
	httpStatus := http.StatusOK

	if dbHealth.Status == "unhealthy" {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	build := buildinfo.Get()
	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Database:  dbHealth,
		Version:   build.Version,
		Commit:    build.Commit,
		BuildDate: build.BuildDate,
	}

	c.JSON(httpStatus, response)
}

// checkDatabaseHealth checks if the database is accessible and responsive
func checkDatabaseHealth(db *gorm.DB) DatabaseHealth {
	start := time.Now()

	sqlDB, err := db.DB()
	if err != nil {
		return DatabaseHealth{
			Status:       "unhealthy",
			ResponseTime: 0,
		}
	}

	// Ping the database
	err = sqlDB.Ping()
	duration := time.Since(start).Milliseconds()

	if err != nil {
		return DatabaseHealth{
			Status:       "unhealthy",
			ResponseTime: float64(duration),
		}
	}

	return DatabaseHealth{
		Status:       "healthy",
		ResponseTime: float64(duration),
	}
}
