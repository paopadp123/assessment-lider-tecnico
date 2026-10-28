import { describe, expect, it } from "vitest";

import { type Interaction, loadContract, match, url } from "./contract.js";
import { issueBody, sim } from "./helpers.js";

/** Prepara el simulador en el estado que pide el contrato. */
async function arrange(state: string) {
  switch (state) {
    case "calm":
      return sim();
    case "issuer-business-error":
      return sim({ issuer: { businessErrorRate: 1 } });
    case "issuer-technical-error":
      return sim({ issuer: { technicalErrorRate: 1 } });
    case "policy-issued-for-order-1": {
      const s = sim();
      await s.app.inject({ method: "POST", url: "/issue", payload: issueBody("order-1") });
      return s;
    }
    default:
      throw new Error(`estado del proveedor sin preparar: ${state}`);
  }
}

/** Lado proveedor: el simulador responde a cada consumidor lo que dice su contrato. */
describe.each(["orchestrator-simulator.json", "issuing-service-simulator.json"])("contrato %s", (name) => {
  const contract = loadContract(name);

  it.each(contract.interactions.map((i): [string, Interaction] => [i.name, i]))("%s", async (_name, interaction) => {
    const s = await arrange(interaction.providerState);
    const { method, headers, body } = interaction.request;

    const res = await s.app.inject({
      method,
      url: url(interaction),
      ...(headers === undefined ? {} : { headers }),
      ...(body === undefined ? {} : { payload: body as object }),
    });

    expect(res.statusCode).toBe(interaction.response.status);
    if (interaction.response.body !== undefined) {
      expect(match(interaction.response.body, res.json(), interaction.exact)).toEqual([]);
    }
  });
});

describe("comparación por forma", () => {
  const example = { policyNumber: "POL-1", data: { items: [{ id: "a" }] }, empty: [] };

  it.each([
    ["misma forma y campos de más", { policyNumber: "POL-9", data: { items: [{ id: "b", x: 1 }] }, empty: [], extra: true }, [], ""],
    ["falta un campo", { data: { items: [] }, empty: [] }, [], "$.policyNumber: falta"],
    ["otro tipo", { policyNumber: 7, data: { items: [] }, empty: [] }, [], "$.policyNumber: se esperaba string, llegó number"],
    ["arreglo vacío exige vacío", { policyNumber: "P", data: { items: [] }, empty: [1] }, [], "$.empty: se esperaba un arreglo vacío"],
    ["valor exacto distinto", { policyNumber: "POL-9", data: { items: [] }, empty: [] }, ["policyNumber"], "policyNumber: se esperaba"],
  ] as const)("%s", (_name, actual, exact, problem) => {
    const problems = match(example, actual, [...exact]).join("; ");
    if (problem === "") {
      expect(problems).toBe("");
    } else {
      expect(problems).toContain(problem);
    }
  });
});
