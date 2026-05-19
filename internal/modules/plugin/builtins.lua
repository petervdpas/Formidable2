-- builtins.lua — Lua-side standard library mounted on the `formidable`
-- global. Evaluated once per Lua state by installFormidable, after the
-- Go-side namespaces (formidable.path, formidable.url, …) are already
-- on the global, so functions here can compose them.
--
-- Stays pure-Lua so plugin authors can read the source without
-- crossing into Go.

local escape_pattern = function(s)
  return (s:gsub("([%%%(%)%.%[%]%+%-%*%?%^%$])", "%%%1"))
end

local fill_template = function(format, vars)
  return (format:gsub("{(%w+)}", function(key)
    return vars[key] or ""
  end))
end

-- ── formidable.rewrite ────────────────────────────────────────────
--
-- Post-render markdown transforms for export plugins. All functions
-- are pure: take markdown in, return rewritten markdown (and any
-- collected side-data) out. No state, no I/O.

formidable.rewrite = {}

-- markdown(md, opts) rewrites the runtime URLs the renderer emits
-- (/api/images/<stem>/<file>, formidable://tpl:data#frag) into target
-- URLs an export plugin wants on disk. Returns (rewritten, images)
-- where `images` is a set { urlencoded_name = true } of every image
-- that survived the image pass — copy these to the export tree.
--
-- opts (all optional — missing config disables that pass):
--   template_stem      — anchors the /api/images/<stem>/ regex
--   image_path_prefix  — replaces "/api/images/<stem>/" in the output
--                        URL (e.g. ".images/")
--   link_path_format   — formidable:// substitute. Placeholders:
--                        {tpl}, {data}, {hash}. Example:
--                        "/{tpl}/{data}{hash}"
function formidable.rewrite.markdown(md, opts)
  opts = opts or {}
  local images = {}

  local stem = opts.template_stem or ""
  local image_prefix = opts.image_path_prefix or ""
  if stem ~= "" and image_prefix ~= "" then
    local stem_pat = escape_pattern(stem)
    md = md:gsub(
      "(!%[[^%]]*%]%()/api/images/" .. stem_pat .. "/([^)]+)%)",
      function(prefix, urlname)
        images[urlname] = true
        return prefix .. image_prefix .. urlname .. ")"
      end)
  end

  local link_fmt = opts.link_path_format or ""
  if link_fmt ~= "" then
    md = md:gsub(
      "(%[[^%]]+%]%()formidable://([^():%s]+):([^%s)#]+)(#?[^%s)]*)%)",
      function(prefix, tpl, data, hash)
        local tslug = formidable.path.stripExt(tpl, ".yaml")
        local dslug = formidable.path.stripExt(data, ".meta.json")
        return prefix .. fill_template(link_fmt, {
          tpl = tslug, data = dslug, hash = hash,
        }) .. ")"
      end)

    md = md:gsub(
      "formidable://([^():%s]+):([^%s)#]+)(#?[^%s)]*)",
      function(tpl, data, hash)
        local tslug = formidable.path.stripExt(tpl, ".yaml")
        local dslug = formidable.path.stripExt(data, ".meta.json")
        return "[" .. tslug .. "/" .. dslug .. "](" .. fill_template(link_fmt, {
          tpl = tslug, data = dslug, hash = hash,
        }) .. ")"
      end)
  end

  return md, images
end
