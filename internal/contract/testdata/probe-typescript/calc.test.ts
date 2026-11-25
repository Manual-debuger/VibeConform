import { expect, test } from "vitest";
import { answer } from "./calc";

test("answer", () => {
  expect(answer()).toBe(42);
});
