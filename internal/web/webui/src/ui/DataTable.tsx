import { JSX, createMemo, createSignal, For, Show } from "solid-js";
import { ChevronUp, ChevronDown, ChevronsUpDown, ChevronLeft, ChevronRight } from "lucide-solid";

export type SortDir = "asc" | "desc";

export interface Column<T> {
  key: string;
  header: JSX.Element;
  cell: (row: T) => JSX.Element;
  sortable?: boolean;
  sortBy?: (row: T) => number | string;
  align?: "left" | "right";
  width?: string;
  /** When true, hide the column on screens < md. */
  mdOnly?: boolean;
  className?: string;
}

interface Props<T> {
  rows: T[];
  columns: Column<T>[];
  rowKey: (row: T) => string | number;
  /** Initial page size; user can change via the footer selector. */
  pageSize?: number;
  /** Page-size options shown in the footer selector. 0 = All. */
  pageSizeOptions?: number[];
  defaultSort?: { key: string; dir: SortDir };
  onRowClick?: (row: T) => void;
  empty?: JSX.Element;
  /** Optional density override; defaults to "comfortable". */
  density?: "comfortable" | "compact";
}

export default function DataTable<T>(props: Props<T>) {
  const [sort, setSort] = createSignal<{ key: string; dir: SortDir } | null>(
    props.defaultSort ?? null,
  );
  const [page, setPage] = createSignal(1);
  // 0 means "All"; otherwise a positive page size.
  const [pageSize, setPageSize] = createSignal(props.pageSize ?? 50);

  const density = () => props.density ?? "comfortable";
  const pageSizeOptions = () => props.pageSizeOptions ?? [25, 50, 100, 0];

  const sorted = createMemo(() => {
    const s = sort();
    const rows = props.rows;
    if (!s) return rows;
    const col = props.columns.find((c) => c.key === s.key);
    if (!col?.sortBy) return rows;
    const dir = s.dir === "asc" ? 1 : -1;
    return rows.slice().sort((a, b) => {
      const av = col.sortBy!(a);
      const bv = col.sortBy!(b);
      if (av < bv) return -1 * dir;
      if (av > bv) return 1 * dir;
      return 0;
    });
  });

  const effectiveSize = () => (pageSize() === 0 ? sorted().length || 1 : pageSize());

  const totalPages = createMemo(() =>
    Math.max(1, Math.ceil(sorted().length / effectiveSize())),
  );

  const pageRows = createMemo(() => {
    if (pageSize() === 0) return sorted();
    const p = Math.min(page(), totalPages());
    const start = (p - 1) * effectiveSize();
    return sorted().slice(start, start + effectiveSize());
  });

  const toggleSort = (key: string) => {
    const cur = sort();
    if (!cur || cur.key !== key) {
      setSort({ key, dir: "desc" });
    } else if (cur.dir === "desc") {
      setSort({ key, dir: "asc" });
    } else {
      setSort(null);
    }
    setPage(1);
  };

  const padY = () => (density() === "compact" ? "py-1.5" : "py-2.5");

  return (
    <div class="rounded-xl border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
      <div class="scroll-pretty overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead class="bg-zinc-50/80 text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:bg-zinc-900/50">
            <tr>
              <For each={props.columns}>
                {(c) => (
                  <th
                    class={
                      "px-3 font-semibold whitespace-nowrap " +
                      padY() +
                      " " +
                      (c.align === "right" ? "text-right " : "") +
                      (c.mdOnly ? "hidden md:table-cell " : "") +
                      (c.className ?? "")
                    }
                    style={c.width ? { width: c.width } : undefined}
                  >
                    <Show
                      when={c.sortable && c.sortBy}
                      fallback={<span>{c.header}</span>}
                    >
                      <button
                        class={
                          "inline-flex items-center gap-1 hover:text-zinc-900 dark:hover:text-zinc-100 " +
                          (sort()?.key === c.key
                            ? "text-emerald-700 dark:text-emerald-400"
                            : "")
                        }
                        onClick={() => toggleSort(c.key)}
                      >
                        {c.header}
                        {sort()?.key === c.key ? (
                          sort()!.dir === "desc" ? (
                            <ChevronDown size={11} />
                          ) : (
                            <ChevronUp size={11} />
                          )
                        ) : (
                          <ChevronsUpDown size={11} class="opacity-40" />
                        )}
                      </button>
                    </Show>
                  </th>
                )}
              </For>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-100 dark:divide-zinc-800/70">
            <Show
              when={pageRows().length}
              fallback={
                <tr>
                  <td colspan={props.columns.length} class="p-0">
                    {props.empty}
                  </td>
                </tr>
              }
            >
              <For each={pageRows()}>
                {(row) => (
                  <tr
                    class={
                      "group " +
                      (props.onRowClick
                        ? "cursor-pointer hover:bg-emerald-50/40 dark:hover:bg-emerald-500/5"
                        : "hover:bg-zinc-50/70 dark:hover:bg-zinc-900/40")
                    }
                    onClick={() => props.onRowClick?.(row)}
                  >
                    <For each={props.columns}>
                      {(c) => (
                        <td
                          class={
                            "px-3 align-middle " +
                            padY() +
                            " " +
                            (c.align === "right" ? "text-right " : "") +
                            (c.mdOnly ? "hidden md:table-cell " : "") +
                            (c.className ?? "")
                          }
                        >
                          {c.cell(row)}
                        </td>
                      )}
                    </For>
                  </tr>
                )}
              </For>
            </Show>
          </tbody>
        </table>
      </div>
      <div class="flex flex-wrap items-center justify-between gap-3 border-t border-zinc-200 px-3 py-2 text-xs text-zinc-500 dark:border-zinc-800">
        <div class="inline-flex items-center gap-2">
          <span>Rows</span>
          <select
            class="h-7 rounded-md border border-zinc-200 bg-white px-1.5 text-xs focus:border-emerald-500 focus:outline-none dark:border-zinc-800 dark:bg-zinc-900"
            value={pageSize()}
            onChange={(e) => {
              setPageSize(Number(e.currentTarget.value));
              setPage(1);
            }}
          >
            <For each={pageSizeOptions()}>
              {(n) => <option value={n}>{n === 0 ? "All" : n}</option>}
            </For>
          </select>
          <span class="tabular-nums">
            <Show when={sorted().length} fallback="0">
              {pageSize() === 0
                ? `${sorted().length}`
                : `${(page() - 1) * effectiveSize() + 1}–${Math.min(
                    page() * effectiveSize(),
                    sorted().length,
                  )}`}
              {" of "}
              {sorted().length}
            </Show>
          </span>
        </div>
        <Show when={pageSize() !== 0 && totalPages() > 1}>
          <div class="inline-flex items-center gap-1">
            <button
              class="grid h-7 w-7 place-items-center rounded-md text-zinc-600 hover:bg-zinc-100 disabled:opacity-30 dark:text-zinc-400 dark:hover:bg-zinc-800"
              disabled={page() <= 1}
              onClick={() => setPage(Math.max(1, page() - 1))}
            >
              <ChevronLeft size={14} />
            </button>
            <span class="tabular-nums">
              {page()} / {totalPages()}
            </span>
            <button
              class="grid h-7 w-7 place-items-center rounded-md text-zinc-600 hover:bg-zinc-100 disabled:opacity-30 dark:text-zinc-400 dark:hover:bg-zinc-800"
              disabled={page() >= totalPages()}
              onClick={() => setPage(Math.min(totalPages(), page() + 1))}
            >
              <ChevronRight size={14} />
            </button>
          </div>
        </Show>
      </div>
    </div>
  );
}
