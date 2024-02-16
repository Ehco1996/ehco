package web

import (
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func NginxLogMiddleware(logger *zap.SugaredLogger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			// 继续处理请求
			err := next(c)

			stop := time.Now()
			latency := stop.Sub(start)
			clientIP := c.RealIP()

			// NGINX 风格的日志格式
			logger.Infof("%s - - \"%s %s %s\" %d %v",
				clientIP,
				c.Request().Method,
				c.Request().RequestURI,
				c.Request().Proto,
				c.Response().Status,
				latency,
			)

			return err
		}
	}
}
