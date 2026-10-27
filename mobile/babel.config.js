module.exports = {
  presets: ['module:@react-native/babel-preset'],
  // Inyecta la configuración de compilación (src/config.ts) desde el entorno al empaquetar. Solo estas
  // variables: cualquier otra del entorno nunca entra al bundle.
  plugins: [['transform-inline-environment-variables', { include: ['DRIVER_API_URL', 'DRIVER_DEVICE_TOKEN', 'DRIVER_PLATE'] }]],
};
