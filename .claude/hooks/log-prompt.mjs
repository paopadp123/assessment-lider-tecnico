// UserPromptSubmit: guarda cada prompt real, tal como se escribió, en docs/ai/prompts-raw.md.
// Insumo de la bitácora de prompts del Reporte de IA. Enmascara secretos evidentes.
import { appendFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";

const SECRET_PATTERNS = [
  /(api[_-]?key|token|secret|password|passwd|pwd)\s*[:=]\s*\S+/gi,
  /\b(gh[pousr]_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9-]{20,}|AIza[0-9A-Za-z_-]{30,})\b/g,
];

let raw = "";
process.stdin.on("data", (chunk) => (raw += chunk));
process.stdin.on("end", () => {
  try {
    const { prompt = "", cwd = process.cwd() } = JSON.parse(raw);
    if (!prompt.trim()) return;

    let clean = prompt;
    for (const pattern of SECRET_PATTERNS) clean = clean.replace(pattern, "<REDACTADO>");

    const root = process.env.CLAUDE_PROJECT_DIR || cwd;
    const dir = join(root, "docs", "ai");
    mkdirSync(dir, { recursive: true });

    const stamp = new Date().toLocaleString("sv-SE", { timeZone: "America/Bogota" }).slice(0, 16);
    const quoted = clean.split(/\r?\n/).map((line) => `> ${line}`).join("\n");
    appendFileSync(join(dir, "prompts-raw.md"), `\n### ${stamp} · Claude Code\n${quoted}\n`, "utf8");
  } catch {
    // Un fallo del registro nunca debe bloquear el trabajo.
  }
});
