import { randomBytes } from "node:crypto";
import { readFile, rename, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

export const dataFile = fileURLToPath(new URL("../../.annotations.json", import.meta.url));

const fields = ["page", "url", "heading", "quote", "comment", "status"];
const statuses = ["open", "resolved"];

export class NotFoundError extends Error {}
export class ValidationError extends Error {}

export async function load(file = dataFile) {
  try {
    return JSON.parse(await readFile(file, "utf8")).annotations;
  } catch (err) {
    if (err.code === "ENOENT") return [];
    throw err;
  }
}

async function save(file, annotations) {
  await writeFile(`${file}.tmp`, `${JSON.stringify({ annotations }, null, 2)}\n`);
  await rename(`${file}.tmp`, file);
}

let queue = Promise.resolve();

function mutate(file, change) {
  const run = queue.then(async () => {
    const annotations = await load(file);
    const result = change(annotations);
    await save(file, annotations);
    return result;
  });
  queue = run.catch(() => {});
  return run;
}

function pick(input) {
  const picked = Object.fromEntries(fields.filter((f) => f in input).map((f) => [f, input[f]]));
  if ("status" in picked && !statuses.includes(picked.status)) {
    throw new ValidationError(`status must be one of ${statuses.join(", ")}`);
  }
  if ("quote" in picked && typeof picked.quote?.exact !== "string") {
    throw new ValidationError("quote.exact must be a string");
  }
  return picked;
}

function find(annotations, id) {
  const annotation = annotations.find((a) => a.id === id);
  if (!annotation) throw new NotFoundError(`no annotation with id ${id}`);
  return annotation;
}

export function create(input, file = dataFile) {
  const picked = pick(input);
  if (!picked.page || !picked.quote) throw new ValidationError("page and quote are required");
  return mutate(file, (annotations) => {
    let id;
    do id = randomBytes(4).toString("hex");
    while (annotations.some((a) => a.id === id));
    const now = new Date().toISOString();
    const annotation = { id, ...picked, comment: picked.comment ?? "", status: picked.status ?? "open", created: now, updated: now };
    annotations.push(annotation);
    return annotation;
  });
}

export function update(id, input, file = dataFile) {
  const picked = pick(input);
  return mutate(file, (annotations) =>
    Object.assign(find(annotations, id), picked, { updated: new Date().toISOString() }),
  );
}

export function remove(id, file = dataFile) {
  return mutate(file, (annotations) => {
    annotations.splice(annotations.indexOf(find(annotations, id)), 1);
  });
}
