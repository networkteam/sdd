import { fileURLToPath } from "node:url";
import { create, load, NotFoundError, remove, update, ValidationError } from "./store.mjs";

const endpoint = "/__sdd-annotations";
const client = fileURLToPath(new URL("./client.js", import.meta.url));

async function readJson(req) {
  let raw = "";
  for await (const chunk of req) raw += chunk;
  return JSON.parse(raw);
}

async function respond(req) {
  const id = decodeURIComponent(req.url.split("?")[0].replace(/^\/+/, ""));
  switch (req.method) {
    case "GET":
      return [200, await load()];
    case "POST":
      return [201, await create(await readJson(req))];
    case "PUT":
      return [200, await update(id, await readJson(req))];
    case "DELETE":
      await remove(id);
      return [204];
    default:
      return [405, { error: `method ${req.method} not allowed` }];
  }
}

// Review comments on the dev server. Hooks do nothing outside `astro dev`, so
// the production build contains none of it.
export default function annotations() {
  let enabled = false;
  return {
    name: "sdd-annotations",
    hooks: {
      "astro:config:setup": ({ command, injectScript }) => {
        enabled = command === "dev";
        if (enabled) injectScript("page", `import ${JSON.stringify(client)};`);
      },
      "astro:server:setup": ({ server, logger }) => {
        if (!enabled) return;
        server.middlewares.use(endpoint, async (req, res) => {
          let status;
          let body;
          try {
            [status, body] = await respond(req);
          } catch (err) {
            status = err instanceof NotFoundError ? 404 : err instanceof ValidationError || err instanceof SyntaxError ? 400 : 500;
            body = { error: err.message };
            if (status === 500) logger.error(err.stack ?? err.message);
          }
          res.statusCode = status;
          if (body === undefined) return res.end();
          res.setHeader("Content-Type", "application/json");
          res.end(JSON.stringify(body));
        });
      },
    },
  };
}
