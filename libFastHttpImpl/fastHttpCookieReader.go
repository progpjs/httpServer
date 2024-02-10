package libFastHttpImpl

import (
	"github.com/progpjs/libHttpServer"
	"github.com/valyala/fasthttp"
	"time"
)

type fastHttpCookie struct {
	fast fasthttp.Cookie
}

func (m *fastHttpCookie) IsHTTPOnly() bool {
	return m.fast.HTTPOnly()
}

func (m *fastHttpCookie) IsSecure() bool {
	return m.fast.Secure()
}

func (m *fastHttpCookie) GetSameSiteType() libHttpServer.CookieSameSite {
	return libHttpServer.CookieSameSite(m.fast.SameSite())
}

func (m *fastHttpCookie) GetKey() string {
	return UnsafeString(m.fast.Key())
}

func (m *fastHttpCookie) GetDomain() string {
	return UnsafeString(m.fast.Domain())
}

func (m *fastHttpCookie) GetValue() string {
	return UnsafeString(m.fast.Value())
}

func (m *fastHttpCookie) GetExpireTime() time.Time {
	return m.fast.Expire()
}

func (m *fastHttpCookie) GetMaxAge() int {
	return m.fast.MaxAge()
}

func cookieToJson(m *fastHttpCookie) map[string]any {
	out := make(map[string]any)

	out["key"] = m.GetKey()
	out["domain"] = m.GetDomain()
	out["value"] = m.GetValue()
	out["maxAge"] = m.GetMaxAge()
	out["expireTime"] = m.GetExpireTime().Unix()
	out["sameSiteType"] = m.GetSameSiteType()
	out["isSecure"] = m.IsSecure()
	out["isHTTPOnly"] = m.IsHTTPOnly()

	return out
}
