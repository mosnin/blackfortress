// Copyright (c) 2026 Probo Inc <hello@probo.com>.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import type { ComponentProps } from "react";

export type BlackFortressLogoProps = ComponentProps<"svg"> & {
  // Render only the fortress mark (square), without the wordmark.
  markOnly?: boolean;
};

// Lime fortress mark: a crenellated wall with an arched gate.
const FORTRESS_MARK_PATH
  = "M2 22V3h4.5v3.5h3V3h5v3.5h3V3H22v19h-7.25v-5.25a2.75 2.75 0 0 0-5.5 0V22Z";

// Black Fortress brand logo: lime fortress mark + "BLACK FORTRESS" wordmark.
// The mark is always lime (#A3E635); the wordmark renders in `currentColor`
// (white on the default dark surface). Size it with a height class (`h-6`).
export function BlackFortressLogo({ markOnly = false, ...props }: BlackFortressLogoProps) {
  if (markOnly) {
    return (
      <svg
        xmlns="http://www.w3.org/2000/svg"
        fill="none"
        viewBox="0 0 24 24"
        role="img"
        aria-label="Black Fortress"
        {...props}
      >
        <path fill="#A3E635" d={FORTRESS_MARK_PATH} />
      </svg>
    );
  }

  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 172 24"
      role="img"
      aria-label="Black Fortress"
      {...props}
    >
      <path fill="#A3E635" d={FORTRESS_MARK_PATH} />
      <text
        x="31"
        y="17.5"
        fill="currentColor"
        fontFamily="inherit"
        fontSize="15"
        fontWeight="700"
        letterSpacing="1"
        textLength="141"
        lengthAdjust="spacingAndGlyphs"
      >
        BLACK
        <tspan fill="#A3E635"> FORTRESS</tspan>
      </text>
    </svg>
  );
}
