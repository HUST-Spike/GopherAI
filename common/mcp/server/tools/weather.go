package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

// wttrResponse maps the subset of wttr.in's j1 format we actually consume.
type wttrResponse struct {
	CurrentCondition []struct {
		TempC         string `json:"temp_C"`
		FeelsLikeC    string `json:"FeelsLikeC"`
		Humidity      string `json:"humidity"`
		WindspeedKmph string `json:"windspeedKmph"`
		WeatherDesc   []struct {
			Value string `json:"value"`
		} `json:"weatherDesc"`
	} `json:"current_condition"`
	NearestArea []struct {
		AreaName []struct {
			Value string `json:"value"`
		} `json:"areaName"`
		Country []struct {
			Value string `json:"value"`
		} `json:"country"`
	} `json:"nearest_area"`
}

func registerWeather(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"get_weather",
		mcp.WithDescription("获取指定城市的实时天气：温度 / 体感 / 湿度 / 风速 / 天气描述。"),
		mcp.WithString("city",
			mcp.Description("城市名称，如 Beijing、上海、Tokyo"),
			mcp.Required(),
		),
	)
	reg.Register(tool, handleGetWeather)
}

func handleGetWeather(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	city, err := stringArg(req, "city")
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://wttr.in/%s?format=j1&lang=zh", city)
	var data wttrResponse
	if err := httpGetJSON(ctx, url, &data); err != nil {
		return nil, fmt.Errorf("get_weather: %w", err)
	}
	if len(data.CurrentCondition) == 0 {
		return nil, fmt.Errorf("get_weather: no data for city %q", city)
	}

	cc := data.CurrentCondition[0]
	temp, _ := strconv.ParseFloat(cc.TempC, 64)
	feels, _ := strconv.ParseFloat(cc.FeelsLikeC, 64)
	humidity, _ := strconv.Atoi(cc.Humidity)
	wind, _ := strconv.ParseFloat(cc.WindspeedKmph, 64)

	location := city
	if len(data.NearestArea) > 0 && len(data.NearestArea[0].AreaName) > 0 {
		location = data.NearestArea[0].AreaName[0].Value
	}
	country := ""
	if len(data.NearestArea) > 0 && len(data.NearestArea[0].Country) > 0 {
		country = data.NearestArea[0].Country[0].Value
	}
	condition := "未知"
	if len(cc.WeatherDesc) > 0 {
		condition = cc.WeatherDesc[0].Value
	}

	text := fmt.Sprintf(
		"位置: %s%s\n天气: %s\n温度: %.1f°C (体感 %.1f°C)\n湿度: %d%%\n风速: %.1f km/h",
		location, formatCountry(country), condition, temp, feels, humidity, wind,
	)
	return textResult(text), nil
}

func formatCountry(c string) string {
	if c == "" {
		return ""
	}
	return ", " + c
}
