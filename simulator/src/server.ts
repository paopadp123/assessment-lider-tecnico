import { buildApp } from "./app.js";
import { ChaosState } from "./chaos.js";
import { loadConfig } from "./config.js";
import { realSleep } from "./deps.js";
import { SeededRandom } from "./random.js";
import { httpDeliver, timerScheduler } from "./webhook.js";

const config = loadConfig(process.env);
const app = buildApp({
  chaos: new ChaosState(config.chaos, Date.now()),
  random: new SeededRandom(config.seed),
  now: Date.now,
  sleep: realSleep,
  scheduler: timerScheduler,
  deliver: httpDeliver,
  webhookUrl: config.webhookUrl,
  webhookSecret: config.webhookSecret,
});

// La semilla queda en el log: con SIM_SEED se puede repetir la misma corrida.
app.log.info({ seed: config.seed, webhookUrl: config.webhookUrl, chaos: config.chaos }, "simulator config");

for (const signal of ["SIGINT", "SIGTERM"] as const) {
  process.once(signal, () => {
    void app.close().then(() => process.exit(0));
  });
}

await app.listen({ host: "0.0.0.0", port: config.port });
