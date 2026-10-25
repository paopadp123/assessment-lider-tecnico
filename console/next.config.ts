import type { NextConfig } from "next";

const config: NextConfig = {
  // Imagen de Docker mínima: Next copia solo lo que el servidor necesita (ver Dockerfile).
  output: "standalone",
  poweredByHeader: false,
};

export default config;
