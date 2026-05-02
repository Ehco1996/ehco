import { createSignal, Show } from "solid-js";
import { KeyRound } from "lucide-solid";
import { authInfo, creds, signIn } from "../store/auth";
import Button from "../ui/Button";
import { Input } from "../ui/Input";
import Logo from "../ui/Logo";

export default function LoginGate() {
  const c0 = creds();
  const [token, setToken] = createSignal(c0.token);
  const [user, setUser] = createSignal(c0.user);
  const [pass, setPass] = createSignal(c0.pass);
  const [busy, setBusy] = createSignal(false);
  const [err, setErr] = createSignal("");

  // If the server doesn't advertise either scheme (e.g. /auth/info
  // hadn't returned yet, or no auth is configured at all), still let
  // the user submit something — at worst they'll see a clear error.
  const showToken = () => authInfo().token || (!authInfo().token && !authInfo().basic);
  const showBasic = () => authInfo().basic;

  const submit = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    const failure = await signIn({
      token: showToken() ? token() : "",
      user: showBasic() ? user() : "",
      pass: showBasic() ? pass() : "",
    });
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

        <Show when={showBasic()}>
          <label class="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
            Username
          </label>
          <Input
            mono
            autofocus
            autocomplete="username"
            placeholder="web_auth_user"
            value={user()}
            onInput={(e) => setUser(e.currentTarget.value)}
          />
          <label class="mb-1 mt-3 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
            Password
          </label>
          <Input
            type="password"
            mono
            autocomplete="current-password"
            placeholder="web_auth_pass"
            value={pass()}
            onInput={(e) => setPass(e.currentTarget.value)}
          />
        </Show>

        <Show when={showToken()}>
          <label
            class={
              "mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400 " +
              (showBasic() ? "mt-3" : "")
            }
          >
            Access token
          </label>
          <Input
            type="password"
            mono
            autofocus={!showBasic()}
            autocomplete="off"
            placeholder="web_token"
            value={token()}
            onInput={(e) => setToken(e.currentTarget.value)}
          />
          <Show when={!showBasic()}>
            <p class="mt-1.5 text-xs text-zinc-500">
              Leave empty if your ehco instance has no token gate.
            </p>
          </Show>
        </Show>

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
