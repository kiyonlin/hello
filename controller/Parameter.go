package controller

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"hello/model"
	"strings"
	"strconv"
)

func ParameterServe() {
	router := gin.Default()
	router.GET("/get", GetParameters)
	router.GET("/set", SetParameters)
	router.Run(":8080")
}

func GetParameters(c *gin.Context) {
	c.String(http.StatusOK, model.ApplicationConfig.ToString())
}

func SetParameters(c *gin.Context) {
	deduction := c.Query("deduction")
	if len(strings.TrimSpace(deduction)) > 0 {
		model.ApplicationConfig.Deduction, _ = strconv.ParseFloat(deduction, 64)
	}
//	c.Query()
//	if len(strings.TrimSpace())
//deduction: 0.800000
//basecarrycost: -0.000010
//channelslot: 100000.000000
//minusdt: 15.000000
//maxusdt: 1500.000000
//env: ace
}