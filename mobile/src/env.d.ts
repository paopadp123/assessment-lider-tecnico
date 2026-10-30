// Variables que babel.config.js inyecta al empaquetar (transform-inline-environment-variables).
declare const process: {
  env: {
    DRIVER_API_URL?: string;
    DRIVER_DEVICE_TOKEN?: string;
    DRIVER_PLATE?: string;
  };
};
