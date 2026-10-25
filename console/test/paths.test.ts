import { describe, expect, it } from "vitest";

import { isOperatorAction, orderPath } from "@/lib/server/paths";

describe("orderPath", () => {
  it("arma la ruta para un UUID", () => {
    expect(orderPath("4f1c2a7e-0000-4000-8000-000000000001", "/calls")).toBe("/orders/4f1c2a7e-0000-4000-8000-000000000001/calls");
  });

  it("rechaza ids que podrían convertirse en otra ruta del orquestador", () => {
    for (const id of ["../ops/breakers", "x", "4f1c2a7e-0000-4000-8000-000000000001/../../auth/login", "%2e%2e"]) {
      expect(orderPath(id)).toBeNull();
    }
  });
});

describe("isOperatorAction", () => {
  it("solo acepta las dos acciones del orquestador", () => {
    expect(isOperatorAction("retry-issuance")).toBe(true);
    expect(isOperatorAction("mark-for-reversal")).toBe(true);
    expect(isOperatorAction("delete")).toBe(false);
  });
});
