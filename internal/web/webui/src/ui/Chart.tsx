import { onMount, onCleanup, createEffect } from "solid-js";
import uPlot from "uplot";
import { theme } from "../store/theme";

export interface Series {
  label: string;
  values: number[];
  stroke: string;
}

export default function Chart(props: {
  timestamps: number[]; // unix seconds
  series: Series[];
  height?: number;
  yFormat?: (v: number) => string;
}) {
  let host!: HTMLDivElement;
  let plot: uPlot | null = null;

  const build = () => {
    if (plot) {
      plot.destroy();
      plot = null;
    }
    const opts: uPlot.Options = {
      width: host.clientWidth || 600,
      height: props.height ?? 240,
      cursor: { drag: { x: true, y: false } },
      scales: { x: { time: true } },
      axes: [
        {},
        {
          values: props.yFormat
            ? (_self, ticks) => ticks.map((t) => props.yFormat!(t))
            : undefined,
        },
      ],
      series: [
        {},
        ...props.series.map((s) => ({
          label: s.label,
          stroke: s.stroke,
          width: 1.5,
          points: { show: false },
        })),
      ],
    };
    const data: uPlot.AlignedData = [
      props.timestamps,
      ...props.series.map((s) => s.values),
    ];
    plot = new uPlot(opts, data, host);
  };

  const resize = () => {
    if (!plot) return;
    plot.setSize({ width: host.clientWidth, height: props.height ?? 240 });
  };

  onMount(() => {
    build();
    window.addEventListener("resize", resize);
  });
  onCleanup(() => {
    window.removeEventListener("resize", resize);
    plot?.destroy();
  });

  // Rebuild when data changes or theme flips (uPlot picks up CSS but axis
  // colours are baked at construction; rebuilding is cheap for the data sizes
  // we display).
  createEffect(() => {
    void props.timestamps;
    void props.series;
    void theme();
    if (host) build();
  });

  return <div ref={host} class="w-full" />;
}
