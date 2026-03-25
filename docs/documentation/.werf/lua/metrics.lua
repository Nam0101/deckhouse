local dict = ngx.shared.reusable_content_metrics

ngx.header["Content-Type"] = "text/plain; charset=utf-8"

if not dict then
  ngx.say("# reusable content metrics are disabled")
  return
end

local metrics = {
  "reusable_content_pages_total",
  "reusable_content_placeholders_total",
  "reusable_content_include_requests_total",
  "reusable_content_render_errors_total",
  "reusable_content_fallbacks_total",
}

for _, name in ipairs(metrics) do
  ngx.say(name, " ", dict:get(name) or 0)
end
