/**
 * The ehco mark. Just a cat emoji — renders the OS-native glyph
 * (Apple's flat cat on macOS / iOS, Noto on Linux, etc.) so it always
 * matches the user's system style. No backdrop, no SVG fuss.
 *
 * `size` controls both the box and the glyph size. We scale the
 * font-size down slightly so the glyph isn't clipped on platforms with
 * tight emoji bounding boxes.
 */
export default function Logo(props: { size?: number; class?: string }) {
  const size = props.size ?? 32;
  return (
    <span
      role="img"
      aria-label="ehco"
      class={`inline-flex shrink-0 items-center justify-center leading-none ${props.class ?? ""}`}
      style={{
        width: `${size}px`,
        height: `${size}px`,
        "font-size": `${Math.round(size * 0.95)}px`,
      }}
    >
      🐱
    </span>
  );
}
