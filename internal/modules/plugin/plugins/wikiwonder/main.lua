-- WikiWonder — render every storage item to Markdown and write it
-- to a target folder on disk.
--
-- Frontmatter handling honours the `plugins:` convention documented
-- in plugin.json. Each plugin reads its own block under that key,
-- so wikiwonder reads plugins.wikiwonder (and Hugo/WordPress/...
-- plugins would read plugins.hugo / plugins.wordpress / ...).
--
-- Per item:
--
--   1. Render markdown via formidable.render.markdown.
--   2. Parse its frontmatter via formidable.fm.parse.
--   3. Look up data.plugins[formidable.plugin.id] (self-describing —
--      no hardcoded "wikiwonder" string in the FM lookup).
--   4. If the block is present:
--        * block.enabled == false  → skip this item entirely.
--        * Output FM = block contents minus the routing keys
--          (`enabled`, `path`). Anything else is passed through.
--        * block.path overrides the default <stem>.md filename and
--          may contain forward slashes for subpath routing.
--   5. If no block is present:
--        * Default: drop the entire (PDF-shaped) frontmatter.
--        * keep_fm_when_no_plugin_block toggle keeps it verbatim.
--   6. Compose output via formidable.fm.build and write to disk.

local function join_path(a, b)
  local last = a:sub(-1)
  if last == "/" or last == "\\" then
    return a .. b
  end
  return a .. "/" .. b
end

local function strip_meta(name)
  return (string.gsub(name, "%.meta%.json$", ""))
end

local function template_stem(t)
  if t.stem and t.stem ~= "" then return t.stem end
  return (string.gsub(t.filename or "", "%.yaml$", ""))
end

local function copy_without(t, keys)
  local out = {}
  local skip = {}
  for _, k in ipairs(keys) do skip[k] = true end
  for k, v in pairs(t) do
    if not skip[k] then out[k] = v end
  end
  return out
end

local function resolve_out_path(target, tpl_stem, base, plugin_path, group_by_template)
  if type(plugin_path) == "string" and plugin_path ~= "" then
    local cleaned = plugin_path
    if cleaned:sub(-3) == ".md" then cleaned = cleaned:sub(1, -4) end
    return join_path(target, cleaned .. ".md")
  end
  if group_by_template then
    return join_path(join_path(target, tpl_stem), base .. ".md")
  end
  return join_path(target, base .. ".md")
end

-- emit_markdown returns the markdown string to write, or nil to
-- signal "skip this item". keep_fm controls the fallback when no
-- plugin block is present.
local function emit_markdown(tpl_filename, datafile, keep_fm)
  local data, body = formidable.render.frontmatter(tpl_filename, datafile)
  if data == nil then
    return formidable.fm.build(nil, body), nil
  end
  local block = formidable.fm.pluginBlock(data)
  if type(block) == "table" then
    if block.enabled == false then return nil, nil end
    local out_fm = copy_without(block, { "enabled", "path" })
    return formidable.fm.build(out_fm, body), block
  end
  if keep_fm then
    return formidable.fm.build(data, body), nil
  end
  return formidable.fm.build(nil, body), nil
end

-- collect_work walks every template, lists its items, and returns
-- the flat (tpl, item) work list. Done up-front so progress.tick
-- gets a known total — gives the frontend a determinate bar.
local function collect_work()
  local work = {}
  for _, t in ipairs(formidable.template.list() or {}) do
    local tpl_filename = t.filename
    if tpl_filename and tpl_filename ~= "" then
      local ok_list, items = pcall(formidable.collection.list, tpl_filename)
      if ok_list then
        for _, item in ipairs(items or {}) do
          if item.filename and item.filename ~= "" then
            table.insert(work, { tpl = t, item = item })
          end
        end
      else
        formidable.log.warn("WikiWonder: list failed", tpl_filename, tostring(items))
      end
    end
  end
  return work
end

local function process_one(unit, target, group_by_template, overwrite, keep_fm)
  local tpl_filename = unit.tpl.filename
  local stem = template_stem(unit.tpl)
  local datafile = unit.item.filename
  local base = strip_meta(datafile)

  local ok_emit, out_md, block = pcall(emit_markdown, tpl_filename, datafile, keep_fm)
  if not ok_emit then
    formidable.log.warn("WikiWonder: render failed", tpl_filename, datafile, tostring(out_md))
    return "failed", nil
  end
  if out_md == nil then
    formidable.log.info("WikiWonder: skipped (enabled=false)", tpl_filename, datafile)
    return "skipped", nil
  end

  local plugin_path
  if type(block) == "table" then plugin_path = block.path end
  local out_path = resolve_out_path(target, stem, base, plugin_path, group_by_template)

  if (not overwrite) and formidable.fs.exists(out_path) then
    formidable.log.info("WikiWonder: skipped (exists)", out_path)
    return "skipped", out_path
  end
  local ok_write, werr = pcall(formidable.fs.write, out_path, out_md)
  if not ok_write then
    formidable.log.error("WikiWonder: write failed", out_path, tostring(werr))
    return "failed", out_path
  end
  formidable.log.info("WikiWonder: wrote", out_path)
  return "wrote", out_path
end

function export(ctx)
  ctx = ctx or {}

  local target = ctx.target_folder
  if type(target) ~= "string" or target == "" then
    formidable.toast.error("WikiWonder: target folder is empty")
    return { ok = false, error = "no_target" }
  end

  local group_by_template = ctx.group_by_template ~= false
  local overwrite = ctx.overwrite ~= false
  local keep_fm = ctx.keep_fm_when_no_plugin_block == true

  formidable.log.info("WikiWonder export starting",
    "target=", target,
    "group_by_template=", tostring(group_by_template),
    "overwrite=", tostring(overwrite),
    "keep_fm_when_no_plugin_block=", tostring(keep_fm))

  local work = collect_work()
  local total = #work
  formidable.progress.tick(0, total, "WikiWonder: starting")

  local total_written, total_failed, total_skipped = 0, 0, 0

  for i, unit in ipairs(work) do
    local status, _ = process_one(unit, target, group_by_template, overwrite, keep_fm)
    if status == "wrote" then total_written = total_written + 1
    elseif status == "failed" then total_failed = total_failed + 1
    else total_skipped = total_skipped + 1 end
    local label = string.format("%s / %s", unit.tpl.stem or "?", unit.item.filename or "?")
    formidable.progress.tick(i, total, label)
  end

  local summary = string.format(
    "WikiWonder: wrote %d, skipped %d, failed %d",
    total_written, total_skipped, total_failed)
  if total_failed > 0 then
    formidable.toast.warn(summary)
  else
    formidable.toast.success(summary)
  end

  return {
    ok = (total_failed == 0),
    target = target,
    written = total_written,
    skipped = total_skipped,
    failed = total_failed,
  }
end
