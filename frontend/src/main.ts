import { Service as System } from "../bindings/github.com/petervdpas/formidable2/internal/modules/system";

async function bootSmoke() {
    const root = await System.GetAppRoot();
    const el = document.getElementById("app")!;
    el.textContent = `Formidable2 — appRoot: ${root}`;
}

bootSmoke().catch((err) => {
    const el = document.getElementById("app")!;
    el.textContent = `Boot failed: ${String(err)}`;
});
