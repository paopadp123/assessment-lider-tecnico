// PostToolUse (Edit/Write): formatea el archivo recién editado para no gastar ciclos en el linter.
import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { join } from "node:path";

const GOFMT_FALLBACK = "C:\\Program Files\\Go\\bin\\gofmt.exe";

let raw = "";
process.stdin.on("data", (chunk) => (raw += chunk));
process.stdin.on("end", () => {
  try {
    const file = JSON.parse(raw)?.tool_input?.file_path ?? "";
    const root = process.env.CLAUDE_PROJECT_DIR || process.cwd();

    if (file.endsWith(".go")) {
      const gofmt = existsSync(GOFMT_FALLBACK) ? GOFMT_FALLBACK : "gofmt";
      execFileSync(gofmt, ["-w", file], { stdio: "ignore" });
    } else if (/\.(ts|tsx)$/.test(file)) {
      const prettier = join(root, "console", "node_modules", ".bin", process.platform === "win32" ? "prettier.cmd" : "prettier");
      if (existsSync(prettier)) execFileSync(prettier, ["--write", file], { stdio: "ignore", shell: true });
    }
  } catch {
    // Si el formateador falla, el CI lo reporta; no se bloquea la edición.
  }
});
