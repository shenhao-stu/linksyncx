package admin

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetClientVersions(c *gin.Context) {
	versions, err := h.settingService.GetClientVersions(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, versions)
}

func (h *SettingHandler) UpdateClientVersions(c *gin.Context) {
	var choices []service.ClientVersionChoice
	if err := c.ShouldBindJSON(&choices); err != nil {
		response.BadRequest(c, "Invalid client versions")
		return
	}
	if err := h.settingService.UpdateClientVersions(c.Request.Context(), choices); err != nil {
		if errors.Is(err, service.ErrInvalidClientVersions) {
			response.BadRequest(c, err.Error())
		} else {
			response.ErrorFrom(c, err)
		}
		return
	}
	h.GetClientVersions(c)
}
