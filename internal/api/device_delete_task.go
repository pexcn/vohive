package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vohive/internal/config"
)

type deviceDeleteStatus struct {
	DeviceID  string    `json:"device_id"`
	State     string    `json:"state"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type deviceDeleteTask struct {
	status deviceDeleteStatus
	done   chan struct{}
}

func (s *Server) startDeviceDelete(id string, retry bool) (*deviceDeleteTask, int, error) {
	s.deviceDeleteMu.Lock()
	defer s.deviceDeleteMu.Unlock()
	for id, task := range s.deviceDeletes {
		if task.status.State != "running" && time.Since(task.status.UpdatedAt) > time.Hour {
			delete(s.deviceDeletes, id)
		}
	}
	prev := s.deviceDeletes[id]
	if prev != nil && prev.status.State == "running" {
		return prev, http.StatusAccepted, nil
	}
	if retry && (prev == nil || prev.status.State != "failed") {
		return nil, http.StatusConflict, fmt.Errorf("没有可重试的删除任务")
	}
	existing, err := config.GetDeviceByID(id)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("读取设备配置失败: %w", err)
	}
	if existing == nil {
		if prev != nil && prev.status.State == "succeeded" {
			return prev, http.StatusAccepted, nil
		}
		return nil, http.StatusNotFound, fmt.Errorf("设备未找到: %s", id)
	}
	if s.deviceDeletes == nil {
		s.deviceDeletes = make(map[string]*deviceDeleteTask)
	}
	task := &deviceDeleteTask{status: deviceDeleteStatus{DeviceID: id, State: "running", UpdatedAt: time.Now()}, done: make(chan struct{})}
	s.deviceDeletes[id] = task
	go s.runDeviceDelete(id, task)
	return task, http.StatusAccepted, nil
}

func (s *Server) runDeviceDelete(id string, task *deviceDeleteTask) {
	err := s.pool.RemoveWorker(id)
	if err == nil || strings.Contains(err.Error(), "设备未找到") {
		err = config.DeleteDeviceInFile(s.configPath, id)
	}
	s.deviceDeleteMu.Lock()
	defer s.deviceDeleteMu.Unlock()
	task.status.State = "succeeded"
	task.status.UpdatedAt = time.Now()
	if err != nil {
		task.status.State, task.status.Error = "failed", err.Error()
	}
	close(task.done)
}

func (s *Server) handleDeviceMgmtDeleteDevice(c *gin.Context) {
	task, status, err := s.startDeviceDelete(deviceIDParam(c), false)
	if err != nil {
		c.JSON(status, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if strings.EqualFold(c.Query("async"), "true") || strings.EqualFold(c.GetHeader("Prefer"), "respond-async") {
		s.writeDeviceDeleteTask(c, http.StatusAccepted, task)
		return
	}
	select {
	case <-c.Request.Context().Done():
		return
	case <-task.done:
	}
	if task.status.State == "failed" {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "message": task.status.Error})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) writeDeviceDeleteTask(c *gin.Context, httpStatus int, task *deviceDeleteTask) {
	s.deviceDeleteMu.Lock()
	result := task.status
	s.deviceDeleteMu.Unlock()
	c.JSON(httpStatus, result)
}

func (s *Server) handleDeviceDeleteStatus(c *gin.Context) {
	s.deviceDeleteMu.Lock()
	task := s.deviceDeletes[deviceIDParam(c)]
	s.deviceDeleteMu.Unlock()
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "未找到设备删除任务"})
		return
	}
	s.writeDeviceDeleteTask(c, http.StatusOK, task)
}

func (s *Server) handleRetryDeviceDelete(c *gin.Context) {
	task, status, err := s.startDeviceDelete(deviceIDParam(c), true)
	if err != nil {
		c.JSON(status, gin.H{"status": "error", "message": err.Error()})
		return
	}
	s.writeDeviceDeleteTask(c, status, task)
}
