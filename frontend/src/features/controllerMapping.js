import { EventsOn } from "/wailsjs/runtime";
import { BeginControllerMapping, ControllerMappingAction } from "/wailsjs/go/main/App.js";
import diagram from "../assets/images/controller-test.svg?raw";

const layouts = {
  xbox: { label: "Xbox", a: "A", b: "B", x: "X", y: "Y", back: "View / Back", start: "Menu / Start" },
  nintendo: { label: "Nintendo", a: "B", b: "A", x: "Y", y: "X", back: "− / Select", start: "+ / Start" },
  playstation: { label: "PlayStation", a: "Cross ✕", b: "Circle ○", x: "Square □", y: "Triangle △", back: "Share / Select", start: "Options / Start" },
};
const names = { dpup:"D-pad up", dpdown:"D-pad down", dpleft:"D-pad left", dpright:"D-pad right", leftshoulder:"Left shoulder", rightshoulder:"Right shoulder", guide:"Home / Guide", leftstick:"Left stick click", rightstick:"Right stick click", leftx:"Left stick → right", lefty:"Left stick → down", rightx:"Right stick → right", righty:"Right stick → down", lefttrigger:"Left trigger", righttrigger:"Right trigger" };
const opposite = { leftx:"Left stick → left", lefty:"Left stick → up", rightx:"Right stick → left", righty:"Right stick → up" };
const positions = { a:"bottom", b:"right", x:"left", y:"top" };
const diagramButtons = { a:"A",b:"B",x:"X",y:"Y",dpup:"DPAD_UP",dpdown:"DPAD_DOWN",dpleft:"DPAD_LEFT",dpright:"DPAD_RIGHT",leftshoulder:"LB",rightshoulder:"RB",back:"BACK",start:"START",leftstick:"L3",rightstick:"R3",lefttrigger:"LT",righttrigger:"RT",guide:"GUIDE" };

export function createControllerMapping({ container, id, name, message, focus, onDone, onCapture, quickStart=false }) {
  let layout="xbox", state=null, pending=false, active=false, disposed=false, finished=false, operationError="", saveAttempted=false, saving=false;
  let prompt, hint, count, progress, actions, saveButton, skipButton, previousButton;
  const errorText = (error) => String(error?.message || error);
  const hideFooter = (hidden) => { const back=container.closest(".controllers-panel")?.querySelector(".controllers-back"); if (back) back.hidden=hidden; };
  const label = (control) => layouts[layout][control] || names[control] || control;
  const unsubscribe = EventsOn("controller:mapping-state", (next) => {
    if (!disposed && active && next.id===id && !saving) {
      update(next);
      if (next.phase==="ready" && !pending && !saveAttempted) { saveAttempted=true; action("save"); }
      else if (next.command && !pending) action(next.command);
    }
  });

  function button(text, handler, className="") {
    const b=document.createElement("button"); b.textContent=text; b.className=className; b.addEventListener("click",handler); return b;
  }
  function setup() {
    container.replaceChildren();
    const heading=document.createElement("h3"); heading.textContent=name;
    const text=document.createElement("p"); text.textContent="Choose the labels printed on your controller. The result will use Virtual Xbox input in games, Steam and RetroBat.";
    const choices=document.createElement("div"); choices.className="controller-mapping-layouts";
    Object.entries(layouts).forEach(([key,value]) => {
      const b=button(value.label,()=>{ layout=key; for (const child of choices.children) child.setAttribute("aria-pressed",String(child===b)); });
      b.setAttribute("aria-pressed",String(layout===key)); choices.append(b);
    });
    const help=document.createElement("p"); help.className="controller-meta"; help.textContent="Follow the glowing control. Release between steps. The finished layout saves automatically. Escape or Cancel stops an unfinished mapping.";
    container.append(heading,text,choices,help,button("Start mapping",start,"controller-test-button"));
    message(""); focus(0);
  }
  async function start() {
    if (pending || disposed) return;
    pending=true; onCapture(true); container.querySelectorAll("button").forEach(b=>b.disabled=true);
    try {
      const next=await BeginControllerMapping(id);
      if (disposed) { await ControllerMappingAction(id,"cancel"); return; }
      active=true; render(); update(next);
    } catch(error) {
      if (!disposed) { onCapture(false); setup(); message(errorText(error),true); }
    } finally { pending=false; }
  }
  function render() {
    hideFooter(false);
    container.innerHTML=`<div class="controller-mapping-progress"><span></span><progress max="1" value="0"></progress></div><div class="controller-diagram controller-mapping-diagram">${diagram}</div><h3 class="controller-mapping-prompt" aria-live="polite"></h3><p class="controller-mapping-hint"></p><button class="controller-mapping-retry" hidden>Retry saving</button><details class="controller-mapping-tools"><summary>More options</summary><div class="controller-test-actions"></div></details>`;
    count=container.querySelector(".controller-mapping-progress span"); progress=container.querySelector("progress");
    prompt=container.querySelector(".controller-mapping-prompt"); hint=container.querySelector(".controller-mapping-hint"); actions=container.querySelector(".controller-test-actions");
    previousButton=button("Redo previous",()=>action("previous")); skipButton=button("Skip this control",()=>action("skip")); saveButton=container.querySelector(".controller-mapping-retry"); saveButton.addEventListener("click",()=>action("save"));
    actions.append(previousButton,skipButton);
    if (!quickStart) {
      const symbols=layout==="playstation" ? {a:"✕",b:"○",x:"□",y:"△"} : layouts[layout];
      for (const [control,part] of Object.entries({a:"A",b:"B",x:"X",y:"Y"})) {
        const group=container.querySelector(`[data-button="${part}"]`), circle=group?.querySelector("circle"); if (!circle) continue;
        const text=document.createElementNS("http://www.w3.org/2000/svg","text"); text.setAttribute("x",circle.getAttribute("cx")); text.setAttribute("y",circle.getAttribute("cy")); text.setAttribute("dy",".35em"); text.setAttribute("text-anchor","middle"); text.setAttribute("class","mapping-face-label"); text.textContent=symbols[control]; group.append(text);
      }
    }
    focus(0);
  }
  function update(next) {
    state=next;
    const ready=next.phase==="ready", failed=next.phase==="error";
    onCapture(!failed);
    count.textContent=ready ? "Mapping complete" : `Control ${Math.min(next.step+1,next.total)} / ${next.total}`;
    progress.max=next.total; progress.value=next.step;
    const target = next.phase==="opposite" ? opposite[next.control] : label(next.control);
    if (failed) prompt.textContent="Mapping paused";
    else if (ready) prompt.textContent=operationError ? "Mapping could not be saved" : "Layout recorded";
    else if (next.phase==="center") prompt.textContent="Release buttons · Center both sticks";
    else if (next.phase.startsWith("release")) prompt.textContent="Release and center";
    else prompt.textContent=positions[next.control] ? quickStart ? `Press the ${positions[next.control]} face button` : `Press ${target} · ${positions[next.control]} face button` : target;
    hint.textContent=failed ? next.message : ready ? "Saving starts automatically. Press the bottom face button to retry if needed." : next.phase==="center" ? "Leave triggers released and hold still for a moment." : next.phase==="release-opposite" ? "Next: move this stick in the opposite direction." : next.canSkip ? "Missing control? Hold the bottom face button for 1.5s to skip." : "Hold the glowing control briefly, then release.";
    container.querySelectorAll("[data-button], [data-stick]").forEach(el=>el.classList.remove("mapping-target"));
    container.querySelector(".mapping-motion-arrow")?.remove();
    if (!ready && !failed && ["press","opposite"].includes(next.phase)) {
      const part=diagramButtons[next.control]; if (part) container.querySelector(`[data-button="${part}"]`)?.classList.add("mapping-target");
      if (["leftx","lefty","rightx","righty"].includes(next.control)) {
        const stick=container.querySelector(`[data-stick="${next.control.startsWith("left") ? "left" : "right"}"]`); stick?.classList.add("mapping-target");
        if (stick) {
          const svg=stick.ownerSVGElement, arrow=document.createElementNS(svg.namespaceURI,"path"), vertical=next.control.endsWith("y"), sign=next.phase==="opposite" ? -1 : 1;
          const x=Number(stick.dataset.centerX), y=Number(stick.dataset.centerY), dx=vertical ? 0 : 25*sign, dy=vertical ? 25*sign : 0;
          arrow.setAttribute("class","mapping-motion-arrow"); arrow.setAttribute("d",`M${x-dx*.5} ${y-dy*.5}l${dx} ${dy}m${-dx*.35-dy*.3} ${-dy*.35+dx*.3}l${dx*.35+dy*.3} ${dy*.35-dx*.3}l${-dx*.35+dy*.3} ${-dy*.35-dx*.3}`); svg.append(arrow);
        }
      }
    }
    message(failed ? next.message : "Escape to cancel · Finished layouts save automatically.",failed);
    previousButton.disabled=pending || failed || next.step===0; skipButton.hidden=ready || failed; skipButton.disabled=pending || !next.canSkip || next.phase==="center" || next.phase.startsWith("release");
    saveButton.hidden=!(ready && operationError); saveButton.disabled=pending || !ready;
    if (operationError) message(operationError,true);
  }
  async function action(kind) {
    if (pending || disposed || !active) return;
    pending=true; operationError=""; if (state) update(state);
    if (kind==="save") {
      saving=true; hideFooter(true); container.setAttribute("aria-busy","true");
      container.innerHTML=`<div class="controller-mapping-saving" role="status"><div class="controller-mapping-spinner" aria-hidden="true"></div><h3>Saving controller mapping…</h3><p>Applying your layout · Virtual Xbox</p></div>`;
      message("Please wait…");
    }
    try {
      const next=await ControllerMappingAction(id,kind);
      if (disposed) return;
      if (kind==="save") { active=false; finished=true; onCapture(false); dispose(); onDone(true); }
      else { pending=false; update(next); }
    } catch(error) { if (!disposed) { operationError=errorText(error); if (saving) { saving=false; container.removeAttribute("aria-busy"); render(); update(state); } message(operationError,true); } }
    finally { pending=false; if (!disposed && state) { actions.querySelectorAll("button").forEach(b=>b.disabled=false); previousButton.disabled=state.step===0 || state.phase==="error"; saveButton.disabled=state.phase!=="ready"; skipButton.disabled=!state.canSkip || state.phase==="center" || state.phase.startsWith("release"); } }
  }
  async function cancel() {
    if (pending) return;
    pending=true;
    try {
      if (active) await ControllerMappingAction(id,"cancel");
      active=false; finished=true; onCapture(false); dispose(); onDone(false);
    } catch(error) { message(errorText(error),true); }
    finally { pending=false; }
  }
  function dispose() {
    if (disposed) return;
    disposed=true; hideFooter(false); container.removeAttribute("aria-busy"); if (typeof unsubscribe==="function") unsubscribe();
    if (active && !finished) ControllerMappingAction(id,"cancel").catch(console.error);
  }
  if (quickStart) { container.textContent="Preparing input mapper…"; start(); } else setup();
  return { cancel, dispose };
}
