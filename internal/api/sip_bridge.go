package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/internal/sipbridge"
	"gorm.io/gorm"
)

func (s *Server) SetSIPBridge(bridge *sipbridge.Bridge) {
	s.sipBridge = bridge
}

func (s *Server) handleSIPBridgeStatus(c *gin.Context) {
	if s.sipBridge == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false, "accounts": []sipbridge.AccountStatus{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "accounts": s.sipBridge.Status()})
}

func (s *Server) sipBridgeAvailable(c *gin.Context) bool {
	if s.sipBridge == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "SIP 短信桥接未启用"})
		return false
	}
	return true
}

func (s *Server) handleSIPBridgePending(c *gin.Context) {
	if !s.sipBridgeAvailable(c) {
		return
	}
	jobs, err := db.ListSIPBridgePending(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法查询 SIP 短信任务"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": jobs})
}

func (s *Server) handleSIPBridgeSMS(c *gin.Context) {
	if !s.sipBridgeAvailable(c) {
		return
	}
	job, err := db.SIPBridgeJob(c.Request.Context(), strings.TrimSpace(c.Param("message_id")))
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "SIP 短信任务不存在"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法查询 SIP 短信任务"})
	default:
		c.JSON(http.StatusOK, job)
	}
}

func (s *Server) handleSIPBridgeRetry(c *gin.Context) {
	if !s.sipBridgeAvailable(c) {
		return
	}
	requeued, err := db.RetrySIPBridgeDelivery(c.Request.Context(), strings.TrimSpace(c.Param("message_id")))
	switch {
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"message": "无法重试 SIP 短信任务"})
	case !requeued:
		c.JSON(http.StatusConflict, gin.H{"message": "只有待投递或投递失败的 SIP 客户端消息可以重试"})
	default:
		c.JSON(http.StatusAccepted, gin.H{"state": "pending"})
	}
}
