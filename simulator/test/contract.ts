import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

/** Un contrato de contracts/ (raíz del repositorio). El formato está en contracts/README.md. */
export interface ContractFile {
  consumer: string;
  provider: string;
  interactions: Interaction[];
}

export interface Interaction {
  name: string;
  providerState: string;
  request: {
    method: "GET" | "POST";
    path: string;
    query?: Record<string, string>;
    headers?: Record<string, string>;
    body?: unknown;
  };
  response: { status: number; body?: unknown };
  exact?: string[];
  consumerExpects: string;
}

const contractsDir = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "contracts");

export function loadContract(name: string): ContractFile {
  const file = JSON.parse(readFileSync(join(contractsDir, name), "utf8")) as ContractFile;
  if (file.interactions.length === 0) {
    throw new Error(`contrato ${name} sin interacciones`);
  }
  return file;
}

/** Ruta con su query, tal como la envía el consumidor. */
export function url(it: Interaction): string {
  const query = new URLSearchParams(it.request.query ?? {}).toString();
  return query === "" ? it.request.path : `${it.request.path}?${query}`;
}

/**
 * Compara actual contra el ejemplo del contrato: los mismos campos con el mismo tipo JSON (actual puede tener campos
 * de más) y el mismo valor en las rutas de exact. Un arreglo vacío en el ejemplo exige un arreglo vacío.
 */
export function match(expected: unknown, actual: unknown, exact: string[] = []): string[] {
  const problems = shape("$", expected, actual);
  for (const path of exact) {
    const want = lookup(expected, path);
    const got = lookup(actual, path);
    if (want === undefined) {
      problems.push(`${path}: está en exact pero no en el ejemplo`);
    } else if (JSON.stringify(want) !== JSON.stringify(got)) {
      problems.push(`${path}: se esperaba ${JSON.stringify(want)}, llegó ${got === undefined ? "nada" : JSON.stringify(got)}`);
    }
  }
  return problems;
}

function shape(path: string, want: unknown, got: unknown): string[] {
  if (kind(want) !== kind(got)) {
    return [`${path}: se esperaba ${kind(want)}, llegó ${kind(got)}`];
  }
  if (Array.isArray(want) && Array.isArray(got)) {
    if (want.length === 0) {
      return got.length === 0 ? [] : [`${path}: se esperaba un arreglo vacío, llegaron ${got.length} elementos`];
    }
    return got.flatMap((item, i) => shape(`${path}[${i}]`, want[0], item));
  }
  if (isObject(want) && isObject(got)) {
    return Object.entries(want).flatMap(([key, value]) =>
      Object.hasOwn(got, key) ? shape(`${path}.${key}`, value, got[key]) : [`${path}.${key}: falta`],
    );
  }
  return [];
}

function kind(value: unknown): string {
  if (value === null) {
    return "null";
  }
  return Array.isArray(value) ? "arreglo" : typeof value;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function lookup(value: unknown, path: string): unknown {
  let current = value;
  for (const key of path.split(".")) {
    if (!isObject(current) || !Object.hasOwn(current, key)) {
      return undefined;
    }
    current = current[key];
  }
  return current;
}
