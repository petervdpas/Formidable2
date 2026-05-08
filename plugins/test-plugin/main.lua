-- `run` is invoked when the user clicks Plugin → Run → Run.
-- Returns the plugin snapshot so you can see Result gating too.
function run(ctx)
  -- 4 log levels — gated by Show Log + Show log as toast
  formidable.log.info("info ping")
  formidable.log.warn("warn ping")
  formidable.log.error("error ping")
  formidable.log.debug("debug ping")

  -- 4 toast levels — always shown
  formidable.toast.info("toast: info")
  formidable.toast.success("toast: success")
  formidable.toast.warn("toast: warn")
  formidable.toast.error("toast: error")

  -- richer log content (still gated by Show Log)
  formidable.log.info("id=", formidable.plugin.id, "mode=", formidable.plugin.mode, "cmd=", formidable.plugin.command)

  for i, f in ipairs(formidable.plugin.form) do
    formidable.log.info(string.format("field %d: %s (%s)", i, f.label or f.key, f.type))
  end

  if ctx and next(ctx) ~= nil then
    formidable.log.info("ctx=", formidable.json.encode(ctx))
  end

  -- Returned value — gated by Show Result.
  return {
    ok = true,
    plugin = formidable.plugin.id,
    mode = formidable.plugin.mode
  }
end
