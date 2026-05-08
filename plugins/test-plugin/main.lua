-- `run` is invoked when the user clicks Plugin → Run → Run.
-- Return any JSON-shaped value (number, string, table, nil).
function run(ctx)
  -- whole snapshot in one shot — manifest fields + parsed form
  formidable.log.info(formidable.json.encode(formidable.plugin))

  -- or pick fields and the ctx the user just filled in
  formidable.log.info("id=", formidable.plugin.id,
                     " name=", formidable.plugin.name,
                     " version=", formidable.plugin.version,
                     " mode=", formidable.plugin.mode,
                     " command=", formidable.plugin.command,
                     " debug=", tostring(formidable.plugin.debug))

  -- iterate the form fields if any
  for i, f in ipairs(formidable.plugin.form) do
    formidable.log.info(string.format("field %d: %s (%s)", i, f.label or
f.key, f.type))
  end

  -- ctx (what the user typed) — only in form mode
  formidable.log.info("ctx=", formidable.json.encode(ctx))

  return formidable.plugin
end
