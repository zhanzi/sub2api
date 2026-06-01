package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type ImageGenerationCleanupService struct {
	imageService *ImageGenerationService
	timingWheel  *TimingWheelService
}

func NewImageGenerationCleanupService(imageService *ImageGenerationService, timingWheel *TimingWheelService) *ImageGenerationCleanupService {
	return &ImageGenerationCleanupService{imageService: imageService, timingWheel: timingWheel}
}

func (s *ImageGenerationCleanupService) Start() {
	if s == nil || s.imageService == nil || s.timingWheel == nil {
		return
	}
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		result, err := s.imageService.CleanupExpired(ctx, 500)
		if err != nil {
			logger.LegacyPrintf("service.image_generation_cleanup", "cleanup failed: %v", err)
			return
		}
		if result != nil && (result.TasksCleaned > 0 || len(result.Errors) > 0) {
			logger.LegacyPrintf("service.image_generation_cleanup", "tasks_cleaned=%d files_deleted=%d errors=%d", result.TasksCleaned, result.FilesDeleted, len(result.Errors))
		}
	}
	s.timingWheel.Schedule("image_generation_cleanup_startup", 5*time.Minute, run)
	s.timingWheel.ScheduleRecurring("image_generation_cleanup_daily", 24*time.Hour, run)
}
