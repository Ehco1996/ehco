/* @refresh reload */
import { render } from "solid-js/web";
import "./store/theme";
import "./index.css";
import App from "./App";

render(() => <App />, document.getElementById("root") as HTMLElement);
