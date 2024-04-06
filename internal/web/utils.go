package web

import "github.com/labstack/echo/v4"

func NewEchoServer() *echo.Echo {
	e := echo.New()
	e.Debug = true
	e.HidePort = true
	e.HideBanner = true
	return e
}
