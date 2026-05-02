import { createSignal, Show } from "solid-js";
import { KeyRound } from "lucide-solid";
import { signIn, token } from "../store/auth";
import Button from "../ui/Button";
import { Input } from "../ui/Input";
import Logo from "../ui/Logo";

export default function LoginGate() {
  const [input, setInput] = createSignal(token());
  const [busy, setBusy] = createSignal(false);
  const [err, setErr] = createSignal("");

  const submit = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    const failure = await signIn(input());
    if (failure) setErr(failure);
    setBusy(false);
  };

  return (
    <div class="fixed inset-0 z-50 grid place-items-center bg-zinc-950/50 p-4 backdrop-blur-sm">
      <form
        onSubmit={submit}
        class="w-full max-w-sm rounded-2xl border border-zinc-200 bg-white p-5 shadow-xl sm:p-6 dark:border-zinc-800 dark:bg-zinc-900"
      >
        <div class="mb-5 flex items-center gap-3">
          <Logo size={40} />
          <div>
            <h1 class="text-base font-semibold leading-none">ehco admin</h1>
            <p class="mt-1 text-xs text-zinc-500">Authenticate to continue</p>
          </div>
        </div>

        <label class="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
          Access token
        </label>
        <Input
          type="password"
          mono
          autofocus
          autocomplete="off"
          placeholder="web_token"
          value={input()}
          onInput={(e) => setInput(e.currentTarget.value)}
        />
        <p class="mt-1.5 text-xs text-zinc-500">
          Leave empty if your ehco instance has no token gate.
        </p>

        <Show when={err()}>
          <div class="mt-3 rounded-md border border-rose-200 bg-rose-50 p-2 text-xs text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/40 dark:text-rose-400">
            {err()}
          </div>
        </Show>

        <Button
          variant="primary"
          size="md"
          loading={busy()}
          leadingIcon={!busy() ? <KeyRound size={13} /> : undefined}
          class="mt-5 w-full"
          type="submit"
        >
          {busy() ? "Verifying" : "Continue"}
        </Button>
      </form>
    </div>
  );
}
