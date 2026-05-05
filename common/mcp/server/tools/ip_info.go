package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

type ipApiResponse struct {
	Status      string  `json:"status"`
	Message     string  `json:"message,omitempty"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Region      string  `json:"region"`
	RegionName  string  `json:"regionName"`
	City        string  `json:"city"`
	ZIP         string  `json:"zip"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`
	Org         string  `json:"org"`
	AS          string  `json:"as"`
	Query       string  `json:"query"`
}

func registerIPInfo(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"get_ip_info",
		mcp.WithDescription("查询 IP 归属地、ISP、经纬度、时区。不传 ip 则查询请求方的公网出口。"),
		mcp.WithString("ip",
			mcp.Description("要查询的 IPv4/IPv6 地址。留空则查请求方公网 IP。"),
		),
	)
	reg.Register(tool, handleIPInfo)
}

func handleIPInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ip := optionalStringArg(req, "ip", "")

	url := "http://ip-api.com/json/"
	if ip != "" {
		url += ip
	}
	url += "?fields=status,message,country,countryCode,region,regionName,city,zip,lat,lon,timezone,isp,org,as,query"

	var data ipApiResponse
	if err := httpGetJSON(ctx, url, &data); err != nil {
		return nil, fmt.Errorf("get_ip_info: %w", err)
	}
	if data.Status != "success" {
		return nil, fmt.Errorf("get_ip_info: lookup failed: %s", data.Message)
	}

	text := fmt.Sprintf(
		"IP: %s\n国家: %s (%s)\n地区: %s, %s\n城市: %s%s\n经纬度: %.4f, %.4f\n时区: %s\nISP: %s\nAS: %s",
		data.Query,
		data.Country, data.CountryCode,
		data.RegionName, data.Region,
		data.City, formatZIP(data.ZIP),
		data.Lat, data.Lon,
		data.Timezone,
		data.ISP,
		data.AS,
	)
	return textResult(text), nil
}

func formatZIP(zip string) string {
	if zip == "" {
		return ""
	}
	return " (" + zip + ")"
}
