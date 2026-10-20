-- people.hue holds one of the sixteen hues of faces, 0 to 15 (data-hue),
-- no longer oklch degrees: rows written before hold index times 22.5,
-- rounded down, which rounds back to the index.
UPDATE people SET hue = ((hue * 16 + 180) / 360) % 16 WHERE hue > 15;
