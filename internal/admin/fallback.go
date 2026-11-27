package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/store"
)

const maxModelFallbacks = 8

type modelFallbackReq struct {
	Model           string   `json:"model"`
	BudgetThreshold int64    `json:"budget_threshold"`
	Fallbacks       []string `json:"fallbacks"`
}

func (a *API) listModelFallbacks(c *gin.Context) {
	var items []*store.ModelFallback
	if err := a.opt.DB.Order("model asc").Find(&items).Error; err != nil {
		fail(c, http.StatusInternalServerError, "fallbacks_query_failed", "查询模型降级链失败")
		return
	}
	ok(c, gin.H{"items": items})
}

func (a *API) replaceModelFallbacks(c *gin.Context) {
	var req struct {
		Items []modelFallbackReq `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误："+err.Error())
		return
	}
	rows, err := normalizeModelFallbacks(req.Items)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	err = a.opt.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&store.ModelFallback{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "fallbacks_save_failed", "保存模型降级链失败："+err.Error())
		return
	}
	ok(c, gin.H{"items": rows})
}

func normalizeModelFallbacks(items []modelFallbackReq) ([]*store.ModelFallback, error) {
	if len(items) > 32 {
		return nil, errors.New("模型降级链最多 32 条")
	}
	rows := make([]*store.ModelFallback, 0, len(items))
	seenModels := make(map[string]bool, len(items))
	for i, item := range items {
		model := strings.TrimSpace(item.Model)
		if model == "" {
			return nil, fmt.Errorf("items[%d].model 不能为空", i)
		}
		if seenModels[model] {
			return nil, fmt.Errorf("items[%d].model 重复：%s", i, model)
		}
		seenModels[model] = true
		if item.BudgetThreshold < 0 {
			return nil, fmt.Errorf("items[%d].budget_threshold 不能为负数", i)
		}
		if len(item.Fallbacks) > maxModelFallbacks {
			return nil, fmt.Errorf("items[%d].fallbacks 最多 %d 个候选模型", i, maxModelFallbacks)
		}
		fallbacks := make([]string, 0, len(item.Fallbacks))
		seenFallbacks := map[string]bool{model: true}
		for j, fallback := range item.Fallbacks {
			fallback = strings.TrimSpace(fallback)
			if fallback == "" {
				return nil, fmt.Errorf("items[%d].fallbacks[%d] 不能为空", i, j)
			}
			if seenFallbacks[fallback] {
				return nil, fmt.Errorf("items[%d].fallbacks[%d] 重复或等于源模型：%s", i, j, fallback)
			}
			seenFallbacks[fallback] = true
			fallbacks = append(fallbacks, fallback)
		}
		if len(fallbacks) == 0 {
			return nil, fmt.Errorf("items[%d].fallbacks 至少需要一个候选模型", i)
		}
		rows = append(rows, &store.ModelFallback{
			Model: model, BudgetThreshold: item.BudgetThreshold, Fallbacks: fallbacks,
		})
	}
	return rows, nil
}
