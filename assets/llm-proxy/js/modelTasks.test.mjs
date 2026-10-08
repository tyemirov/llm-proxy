// @ts-check
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  MODEL_TASKS,
  defaultSelectedTasks,
  offeringMatchesTasks,
  offeringMatchesTasksWithModalities,
  reconcileSelectedTasks,
  tasksForOffering,
  toggleSelectedTask,
} from "./modelTasks.js";

const textOffering = { capabilities: ["text"] };
const imageOffering = { capabilities: ["image_generation"] };
const multimodalOffering = { capabilities: ["text", "image_generation"] };
const visionOffering = { capabilities: ["text", "image_input"] };

describe("defaultSelectedTasks", () => {
  it("prefers Text when Text is available", () => {
    assert.deepEqual(defaultSelectedTasks(MODEL_TASKS), ["text"]);
  });
  it("selects the first available task when Text is unavailable", () => {
    const tasks = MODEL_TASKS.filter((task) => task.id !== "text");
    assert.deepEqual(defaultSelectedTasks(tasks), [tasks[0].id]);
  });
  it("selects nothing when no tasks are available", () => {
    assert.deepEqual(defaultSelectedTasks([]), []);
  });
});

describe("reconcileSelectedTasks", () => {
  it("preserves available selections and removes unavailable selections", () => {
    const tasks = MODEL_TASKS.filter((task) => ["text", "vision"].includes(task.id));
    assert.deepEqual(reconcileSelectedTasks(["text", "transcription"], tasks), ["text"]);
  });
  it("keeps no selection when no selected task remains available", () => {
    const tasks = MODEL_TASKS.filter((task) => ["vision", "transcription"].includes(task.id));
    assert.deepEqual(reconcileSelectedTasks(["text"], tasks), []);
  });
});

describe("toggleSelectedTask", () => {
  it("adds an available task to the selection", () => {
    assert.deepEqual(toggleSelectedTask(["text"], "image_generation", MODEL_TASKS), ["text", "image_generation"]);
  });
  it("releases a pressed task while other tasks remain pressed", () => {
    assert.deepEqual(toggleSelectedTask(["text", "image_generation"], "text", MODEL_TASKS), ["image_generation"]);
  });
  it("releases the final pressed task", () => {
    assert.deepEqual(toggleSelectedTask(["text"], "text", MODEL_TASKS), []);
  });
  it("ignores tasks outside the available inventory", () => {
    assert.deepEqual(toggleSelectedTask(["text"], "unknown", MODEL_TASKS), ["text"]);
  });
});

describe("offeringMatchesTasks", () => {
  it("matches one offering that supports every selected task", () => {
    assert.equal(offeringMatchesTasks(multimodalOffering, ["text", "image_generation"]), true);
  });
  it("rejects one offering that supports only one selected task", () => {
    assert.equal(offeringMatchesTasks(textOffering, ["text", "image_generation"]), false);
    assert.equal(offeringMatchesTasks(imageOffering, ["text", "image_generation"]), false);
  });
  it("matches across providers only on the same offering", () => {
    const providerText = tasksForOffering(textOffering).map((task) => task.id);
    const providerImage = tasksForOffering(imageOffering).map((task) => task.id);
    assert.ok(providerText.includes("text") && !providerText.includes("image_generation"));
    assert.ok(providerImage.includes("image_generation") && !providerImage.includes("text"));
    assert.equal(offeringMatchesTasks(textOffering, ["text", "image_generation"]), false);
    assert.equal(offeringMatchesTasks(imageOffering, ["text", "image_generation"]), false);
  });
  it("excludes vision-only offerings from a Text plus Images selection", () => {
    assert.ok(tasksForOffering(visionOffering).some((task) => task.id === "vision"));
    assert.equal(offeringMatchesTasks(visionOffering, ["text", "image_generation"]), false);
  });
});

describe("offeringMatchesTasksWithModalities", () => {
  it("matches input and output within one selected task", () => {
    assert.equal(offeringMatchesTasksWithModalities(multimodalOffering, ["text", "image_generation"], "text", "image"), true);
  });
  it("rejects input and output combined from different tasks", () => {
    assert.equal(offeringMatchesTasksWithModalities(multimodalOffering, ["text", "image_generation"], "image", "text"), false);
  });
  it("applies AND matching before input and output matching", () => {
    assert.equal(offeringMatchesTasksWithModalities(textOffering, ["text", "image_generation"], "", ""), false);
  });
});
