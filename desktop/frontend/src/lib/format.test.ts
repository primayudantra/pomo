import { expect, test } from "vitest";
import { mmss } from "./format";

test("mmss pads seconds", () => {
	expect(mmss(1500)).toBe("25:00");
	expect(mmss(65)).toBe("1:05");
	expect(mmss(-3)).toBe("0:00");
});
