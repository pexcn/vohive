package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vowifi-go/runtimehost/voicehost"
)

func (s *Server) handleVoiceControl(c *gin.Context) {
	if s.voiceGW == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "voice unavailable"})
		return
	}
	agent, err := s.voiceGW.CallAgent(c.Param("device_id"))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	if c.Request.Method == http.MethodPost {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	}
	action, id := c.Param("action"), c.Param("call_id")
	var result any
	ctx := c.Request.Context()
	switch action {
	case "calls":
		result = agent.Calls()
	case "dial", "answer":
		var req voicehost.CallRequest
		if err = c.ShouldBindJSON(&req); err != nil {
			break
		}
		callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if action == "dial" {
			result, err = agent.Dial(callCtx, req)
		} else {
			result, err = agent.Answer(callCtx, id, req)
		}
	case "hangup":
		err = agent.Hangup(ctx, id)
	case "hold", "resume":
		err = agent.Hold(ctx, id, action == "hold")
	case "dtmf":
		var req struct {
			Digits   string `json:"digits"`
			Duration int    `json:"duration_ms"`
		}
		if err = c.ShouldBindJSON(&req); err == nil {
			err = agent.SendDTMF(ctx, id, req.Digits, req.Duration)
		}
	case "media":
		if c.Request.Method == http.MethodGet {
			readCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			result, err = agent.ReadMedia(readCtx, id)
		} else {
			var frame voicehost.MediaFrame
			if err = c.ShouldBindJSON(&frame); err == nil {
				err = agent.WriteMedia(ctx, id, frame)
			}
		}
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown voice operation"})
		return
	}
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "call": result})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}
func (s *Server) registerVoiceControlRoutes(api *gin.RouterGroup) {
	// All routes inherit the existing authenticated API middleware. Encoded media
	// is exchanged with a client over that API, never an unauthenticated RTP port.
	api.GET("/voice/:device_id/calls", func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "action", Value: "calls"})
		s.handleVoiceControl(c)
	})
	api.POST("/voice/:device_id/dial", func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "action", Value: "dial"})
		s.handleVoiceControl(c)
	})
	api.POST("/voice/:device_id/:call_id/:action", s.handleVoiceControl)
	api.GET("/voice/:device_id/:call_id/media", func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "action", Value: "media"})
		s.handleVoiceControl(c)
	})
}
