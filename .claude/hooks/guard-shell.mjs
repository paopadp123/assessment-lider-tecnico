// PreToolUse (Bash/PowerShell): convierte reglas de la prueba y de la política de IA en controles automáticos.
// Salida con código 2 = bloquea el comando y le explica el motivo a Claude.
import { execSync } from "node:child_process";

const UPSTREAM_OWNERS = ["gothinkster", "reck1ess", "mauron85"];
const MY_OWNER = "paopadp123";
const SECRET_FILE = /(^|\/)(\.env(\..+)?|.*\.(pem|key|pfx|p12)|id_rsa.*|secrets?\.json)$/i;

const block = (reason) => {
  process.stderr.write(`BLOQUEADO por hook: ${reason}\n`);
  process.exit(2);
};

let raw = "";
process.stdin.on("data", (chunk) => (raw += chunk));
process.stdin.on("end", () => {
  let command = "";
  try {
    command = JSON.parse(raw)?.tool_input?.command ?? "";
  } catch {
    return;
  }

  // 1. Descalificador: PR contra un repositorio de terceros.
  //    En un fork, `gh pr create` apunta al repo original por defecto: exigir --repo explícito propio.
  if (/\bgh\s+pr\s+create\b/.test(command) && !new RegExp(`--repo[ =]${MY_OWNER}/`).test(command)) {
    block(`gh pr create debe llevar --repo ${MY_OWNER}/<repo>. En un fork, el destino por defecto es el repositorio original (descalificador de la prueba).`);
  }
  const touchesUpstream = UPSTREAM_OWNERS.some((owner) => command.includes(`${owner}/`));
  if (touchesUpstream && /\b(gh\s+(pr|issue)\s+(create|comment|review)|git\s+push)\b/.test(command)) {
    block("el comando escribe en un repositorio de terceros. Solo se trabaja sobre los forks propios.");
  }

  // 2. Nunca forzar push sobre ramas compartidas.
  if (/\bgit\s+push\b.*(--force|-f\b)/.test(command) && /\b(main|dev)\b/.test(command)) {
    block("push forzado sobre main/dev.");
  }

  // 3. Secretos: revisar lo que está en stage antes de un commit.
  if (/\bgit\s+commit\b/.test(command)) {
    try {
      const staged = execSync("git diff --cached --name-only", { encoding: "utf8" }).split("\n").filter(Boolean);
      const offending = staged.filter((file) => SECRET_FILE.test(file) && !file.endsWith(".env.example"));
      if (offending.length) block(`archivos con posibles secretos en el commit: ${offending.join(", ")}`);
    } catch {
      // Fuera de un repositorio git no hay nada que revisar.
    }
  }
});
