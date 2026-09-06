// mmss formats a second count as "M:SS". Negative input clamps to "0:00".
export const mmss = (s: number): string => {
  const t = Math.max(0, Math.floor(s));
  return `${Math.floor(t / 60)}:${String(t % 60).padStart(2, "0")}`;
};
