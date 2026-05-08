-- `run` is invoked when the user clicks Plugin → Run → Run.
-- Return any JSON-shaped value (number, string, table, nil).
function run(ctx)
  formidable.log.info("hello from plugin!")
  return { ok = true }
end
