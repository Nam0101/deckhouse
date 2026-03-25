local cjson = require("cjson.safe")

local M = {}

local placeholder_pattern = [[<script\s+type=(["'])application/x%-module%-include\1\s*>(.-)</script>]]
local static_attr_pattern = [[(src|href)=("|\')(\./)?static/([^"\']+)("|\')]]
local markdown_link_pattern = [[%[([^%]]+)%]%(([^)]+)%)]]

local allowed_channels = {
  alpha = true,
  beta = true,
  ["early-access"] = true,
  stable = true,
  ["rock-solid"] = true,
  latest = true,
}

local function increment_counter(name, value)
  local dict = ngx.shared.reusable_content_metrics
  if not dict then
    return
  end

  local ok, err = dict:incr(name, value or 1, 0)
  if not ok and err then
    ngx.log(ngx.WARN, "failed to increment reusable content metric ", name, ": ", err)
  end
end

local function copy_headers(headers)
  for k, v in pairs(headers or {}) do
    local key = string.lower(k)
    if key ~= "content-length" and key ~= "transfer-encoding" and key ~= "connection" then
      ngx.header[k] = v
    end
  end
end

local function make_html_response(res, body)
  ngx.status = res.status
  copy_headers(res.header)
  ngx.header["Content-Length"] = nil
  ngx.print(body or res.body or "")
  return ngx.exit(res.status)
end

local function trim(value)
  if not value then
    return nil
  end

  return (value:gsub("^%s+", ""):gsub("%s+$", ""))
end

local function to_html_fallback(value)
  if not value or value == "" then
    return ""
  end

  local replaced = value:gsub(markdown_link_pattern, [[<a href="%2">%1</a>]])
  return "<div class=\"reusable-content-fallback\">" .. replaced .. "</div>"
end

local function normalize_prefix(prefix)
  local value = prefix or ""
  if value == "" then
    return ""
  end

  if not value:match("^/") then
    value = "/" .. value
  end

  return value:gsub("/+$", "")
end

local function build_include_url(cfg, module_prefix)
  local artifact = cfg.artifact
  if not artifact or not artifact:match("^[a-z0-9-][a-z0-9-./]*%.md$") then
    return nil, "invalid artifact"
  end

  if artifact:find("%.ru%.md$") then
    return nil, "artifact should not include language suffix"
  end

  local module_name = cfg.module
  if not module_name or not module_name:match("^[a-z0-9-]+$") then
    return nil, "invalid module"
  end

  local channel = cfg.channel or "stable"
  if not allowed_channels[channel] then
    return nil, "invalid channel"
  end

  local html_artifact = artifact:gsub("%.md$", ".html")
  local base = normalize_prefix(module_prefix)

  return string.format("%s/%s/%s/partials/%s", base, module_name, channel, html_artifact), nil
end

local function rewrite_static_links(body, cfg, public_prefix)
  local prefix = string.format(
    "%s/%s/%s/partials/static/",
    normalize_prefix(public_prefix),
    cfg.module,
    cfg.channel or "stable"
  )

  return (body:gsub(static_attr_pattern, function(attr, quote_open, _dot, path, quote_close)
    return string.format('%s=%s%s%s', attr, quote_open, prefix .. path, quote_close)
  end))
end

local function render_placeholder(script_body, fetch_prefix, public_prefix)
  increment_counter("reusable_content_placeholders_total", 1)

  local decoded, decode_err = cjson.decode(trim(script_body))
  if not decoded then
    increment_counter("reusable_content_render_errors_total", 1)
    ngx.log(ngx.WARN, "failed to decode module include placeholder: ", tostring(decode_err))
    return ""
  end

  local include_url, url_err = build_include_url(decoded, fetch_prefix)
  if not include_url then
    increment_counter("reusable_content_render_errors_total", 1)
    ngx.log(ngx.WARN, "invalid module include placeholder: ", tostring(url_err))
    if decoded.onError == "fallback" then
      increment_counter("reusable_content_fallbacks_total", 1)
      return to_html_fallback(decoded.fallback)
    end
    return ""
  end

  increment_counter("reusable_content_include_requests_total", 1)
  local res = ngx.location.capture(include_url)
  if not res or res.status >= 400 then
    increment_counter("reusable_content_render_errors_total", 1)
    ngx.log(ngx.WARN, "failed to load reusable content from ", include_url, ", status=", res and res.status or "nil")
    if decoded.onError == "fallback" then
      increment_counter("reusable_content_fallbacks_total", 1)
      return to_html_fallback(decoded.fallback)
    end
    return ""
  end

  return rewrite_static_links(res.body or "", decoded, public_prefix)
end

function M.render(raw_location, fetch_prefix, public_prefix)
  local res = ngx.location.capture(raw_location)
  if not res then
    increment_counter("reusable_content_render_errors_total", 1)
    return ngx.exit(ngx.HTTP_INTERNAL_SERVER_ERROR)
  end

  local content_type = (res.header and (res.header["Content-Type"] or res.header["content-type"])) or ""
  local body = res.body or ""
  if not content_type:find("text/html", 1, true) or not body:find("application/x-module-include", 1, true) then
    return make_html_response(res, body)
  end

  increment_counter("reusable_content_pages_total", 1)
  local rendered = body:gsub(placeholder_pattern, function(_quote, script_body)
    return render_placeholder(script_body, fetch_prefix, public_prefix)
  end)

  return make_html_response(res, rendered)
end

return M
