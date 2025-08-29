package observability

import (
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

func InitMetrics() {}

func Handler() http.Handler {
	return promhttp.Handler()
}
