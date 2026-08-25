// Hand-verification snippets for the mycorrhizal-query-string-auth-material
// rule (issue #370).
package tests

func tokenFromQueryIsCaught(c *Context) string {
	// ruleid: mycorrhizal-query-string-auth-material
	return c.Query("token")
}

func apiKeyFromQueryIsCaught(c *Context) string {
	// ruleid: mycorrhizal-query-string-auth-material
	return c.Query("api_key")
}

func secretFromDefaultQueryIsCaught(c *Context) string {
	// ruleid: mycorrhizal-query-string-auth-material
	return c.DefaultQuery("secret", "")
}

func unrelatedQueryParamIsClean(c *Context) string {
	// ok: mycorrhizal-query-string-auth-material
	return c.Query("search")
}

func suppressedOAuthCodeIsClean(c *Context) string {
	// ok: mycorrhizal-query-string-auth-material
	return c.Query("code") // nosemgrep: mycorrhizal-query-string-auth-material
}
