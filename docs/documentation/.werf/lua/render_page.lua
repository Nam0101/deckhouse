local reusable_content = require("reusable_content")

local raw_uri = ngx.var.reusable_content_raw_uri
if not raw_uri or raw_uri == "" then
  ngx.log(ngx.ERR, "reusable content raw uri is not configured")
  return ngx.exit(ngx.HTTP_INTERNAL_SERVER_ERROR)
end

return reusable_content.render(
  raw_uri,
  ngx.var.reusable_content_fetch_prefix or "",
  ngx.var.reusable_content_public_prefix or ""
)
