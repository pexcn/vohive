package api

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/pkg/smscodec"
)

func (s *Server) handleQueueSMS(c *gin.Context) {
	var req struct {
		DeviceID string `json:"device_id"`
		IMSI     string `json:"imsi"`
		Key      string `json:"idempotency_key"`
		Phone    string `json:"phone"`
		Message  string `json:"message"`
		Encoding string `json:"encoding"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	encoding, err := smscodec.NormalizeSMSEncoding(req.Encoding)
	phone := strings.TrimPrefix(req.Phone, "+")
	valid := phone != "" && len(phone) <= 20
	for _, v := range phone {
		if v < '0' || v > '9' {
			valid = false
		}
	}
	if err != nil || !valid || !utf8.ValidString(req.Message) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid SMS encoding, recipient or text"})
		return
	}
	if req.DeviceID == "" || req.IMSI == "" || req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id, imsi and idempotency_key are required"})
		return
	}
	row, err := db.EnqueueSMS(req.DeviceID, req.IMSI, req.Key, req.Phone, req.Message, string(encoding))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, row)
}
func (s *Server) handleSMSQueueStatus(c *gin.Context) {
	row, err := db.GetSMSQueue(c.Param("queue_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue entry unavailable"})
		return
	}
	c.JSON(http.StatusOK, row)
}
