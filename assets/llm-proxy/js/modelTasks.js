// @ts-check

/**
 * @typedef {'text'|'image'|'audio'|'video'} Modality
 * @typedef {{id:string,label:string,description:string,requires:string[],inputs:Modality[],outputs:Modality[],required:Modality[],defaultDomain:import('./types.d.js').CapabilityDomain|null}} ModelTask
 */

/** One task describes one supported transformation, not every combination of model modalities. @type {ModelTask[]} */
export const MODEL_TASKS = [
  {id:'text',label:'Text',description:'Generate text',requires:['text'],inputs:['text'],outputs:['text'],required:['text'],defaultDomain:'text'},
  {id:'vision',label:'Vision',description:'Understand images',requires:['text','image_input'],inputs:['text','image'],outputs:['text'],required:['text','image'],defaultDomain:'text'},
  {id:'image_generation',label:'Images',description:'Generate images',requires:['image_generation'],inputs:['text'],outputs:['image'],required:['text'],defaultDomain:null},
  {id:'image_editing',label:'Editing',description:'Edit images',requires:['image_editing'],inputs:['text','image'],outputs:['image'],required:['text','image'],defaultDomain:null},
  {id:'transcription',label:'Transcribe',description:'Transcribe audio',requires:['dictation'],inputs:['audio'],outputs:['text'],required:['audio'],defaultDomain:'transcription'},
  {id:'speech_generation',label:'Speech',description:'Generate speech',requires:['speech_generation'],inputs:['text'],outputs:['audio'],required:['text'],defaultDomain:'speech'},
  {id:'video_analysis',label:'Video analysis',description:'Understand video',requires:['text','video_input'],inputs:['text','video'],outputs:['text'],required:['video'],defaultDomain:'text'},
  {id:'video_generation',label:'Video',description:'Generate video',requires:['video_generation'],inputs:['text'],outputs:['video'],required:['text'],defaultDomain:null},
  {id:'audio_understanding',label:'Listen',description:'Understand audio',requires:['text','audio_input'],inputs:['text','audio'],outputs:['text'],required:['text','audio'],defaultDomain:'text'},
  {id:'speech_conversion',label:'Voice conversion',description:'Convert a voice',requires:['speech_conversion'],inputs:['audio'],outputs:['audio'],required:['audio'],defaultDomain:null},
  {id:'voice_extraction',label:'Voice extraction',description:'Extract a voice',requires:['voice_extraction'],inputs:['text','audio'],outputs:['audio'],required:['text','audio'],defaultDomain:null},
  {id:'audio_diarization',label:'Speakers',description:'Identify speakers',requires:['audio_diarization'],inputs:['audio'],outputs:['text'],required:['audio'],defaultDomain:null},
  {id:'audio_alignment',label:'Alignment',description:'Align audio and text',requires:['audio_alignment'],inputs:['text','audio'],outputs:['text'],required:['text','audio'],defaultDomain:null},
  {id:'subtitle_creation',label:'Subtitles',description:'Create subtitles',requires:['subtitle_creation'],inputs:['text','audio'],outputs:['text'],required:['audio'],defaultDomain:null},
  {id:'pronunciation_dictionary_creation',label:'Dictionary',description:'Create a pronunciation dictionary',requires:['pronunciation_dictionary_creation'],inputs:['text'],outputs:['text'],required:['text'],defaultDomain:null},
];

export const MODALITY_LABELS = Object.freeze({text:'Text',image:'Image',audio:'Audio',video:'Video'});
const ICON_PATHS = Object.freeze({
  text:'<path d="M4 5h16M4 10h16M4 15h10M4 20h13"/>',
  image:'<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8" cy="8" r="1.5"/><path d="m3 17 6-6 4 4 3-3 5 5"/>',
  audio:'<path d="M3 10v4m4-8v12m5-16v20m5-16v12m4-8v4"/>',
  video:'<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M7 3v18M17 3v18M3 8h4m-4 8h4M17 8h4m-4 8h4"/>',
});

/** @param {{capabilities:string[]}} offering @returns {ModelTask[]} */
export function tasksForOffering(offering) {
  return MODEL_TASKS.filter(task => task.requires.every(capability =>
    offering.capabilities.includes(capability) || (capability === 'dictation' && offering.capabilities.includes('audio_transcription')),
  )).map(task => task.id === 'video_generation' && offering.capabilities.includes('image_input')
    ? {...task, inputs: /** @type {Modality[]} */ (['text','image'])} : task);
}

/** @param {Modality} modality @param {'input'|'output'} direction */
function modalityIcon(modality, direction) {
  const label = `${MODALITY_LABELS[modality]} ${direction}`;
  return `<span class="model-modality" role="img" aria-label="${label}" title="${label}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICON_PATHS[modality]}</svg></span>`;
}

/** @type {Readonly<Record<string, string>>} */
const TASK_ICON_PATHS = Object.freeze({
  text:'<path d="M3 5h11M3 10h8M3 15h5m3 6 1-5L19 8l3 3-8 8-4 1Zm7-10 3 3"/>',
  vision:'<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>',
  image_generation:'<path d="M13 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-8M3 17l6-6 4 4 3-3 5 5M18 1l1.5 4.5L24 7l-4.5 1.5L18 13l-1.5-4.5L12 7l4.5-1.5Z"/><circle cx="7" cy="7" r="1"/>',
  image_editing:'<path d="M13 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-6M3 17l6-6 3 3m2-2 1-4 5-5 3 3-5 5-4 1Zm4-7 3 3"/><circle cx="7" cy="7" r="1"/>',
  transcription:'<path d="M2 10v4m3-7v10m3-13v16m3-12v8M15 6h7m-7 6h7m-7 6h7"/>',
  speech_generation:'<path d="m3 9 5 0 5-5v16l-5-5H3ZM16 8a6 6 0 0 1 0 8m3-11a10 10 0 0 1 0 14"/>',
  video_analysis:'<rect x="2" y="3" width="15" height="15" rx="2"/><path d="M6 3v15M2 8h4m-4 5h4m13 6 4 4"/><circle cx="16" cy="16" r="5"/>',
  video_generation:'<path d="M12 3H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h16V13M6 3v18M2 8h4m-4 8h4M18 1l1.5 4.5L24 7l-4.5 1.5L18 13l-1.5-4.5L12 7l4.5-1.5Z"/>',
  audio_understanding:'<path d="M7 9a5 5 0 0 1 10 0c0 4-5 4-5 8a3 3 0 0 1-6 0m5-8a1 1 0 0 1 2 0c0 2-3 2-3 5"/>',
  speech_conversion:'<path d="M2 9v6m3-9v12m3-8v4m8-5v6m3-9v12m3-8v4M9 5h6l-2-2m2 16H9l2 2"/>',
  voice_extraction:'<path d="M2 10v4m3-8v12m3-10v8M11 12h10m-4-4 4 4-4 4"/>',
  audio_diarization:'<circle cx="8" cy="7" r="3"/><path d="M2 21v-3a6 6 0 0 1 12 0v3m2-17a3 3 0 0 1 0 6m1 4a5 5 0 0 1 5 5v2"/>',
  audio_alignment:'<path d="M3 4h18M3 20h18M5 9v6m4-8v10m4-6v2m4-5v8m4-6v4"/>',
  subtitle_creation:'<rect x="2" y="4" width="20" height="16" rx="2"/><path d="M6 13h5m2 0h5M6 16h12"/>',
  pronunciation_dictionary_creation:'<path d="M12 5v16M2 3c4-1 7 0 10 2 3-2 6-3 10-2v16c-4-1-7 0-10 2-3-2-6-3-10-2Z"/>',
});

/** @param {ModelTask} task */
function renderTaskSymbol(task) {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${TASK_ICON_PATHS[task.id]}</svg>`;
}

/** @param {ModelTask} task */
export function renderTaskIcon(task) {
  return `<span class="model-task-icon" role="img" aria-label="${task.description}" title="${task.description}">${renderTaskSymbol(task)}<span class="model-task-caption" aria-hidden="true">${task.description}</span></span>`;
}

/** @param {ModelTask} task */
export function renderTaskFlow(task) {
  return `<span class="model-flow">${task.inputs.map(input=>modalityIcon(input,'input')).join('')}<span class="model-flow-arrow" aria-hidden="true">→</span>${task.outputs.map(output=>modalityIcon(output,'output')).join('')}<span class="model-flow-caption" aria-hidden="true">${task.inputs.map(input=>MODALITY_LABELS[input]+' input').join(' + ')} → ${task.outputs.map(output=>MODALITY_LABELS[output]+' output').join(' + ')}</span></span>`;
}

/** @param {{capabilities:string[]}} offering */
export function renderTaskDetails(offering) {
  return `<section data-model-tasks><h4>Supported tasks</h4><ul class="model-task-details">${tasksForOffering(offering).map(task=>{
    const optional = task.inputs.filter(input=>!task.required.includes(input));
    return `<li><strong>${task.description}</strong>${renderTaskFlow(task)}<small>Required input: ${task.required.map(input=>MODALITY_LABELS[input]).join(', ')}.${optional.length ? ` Optional input: ${optional.map(input=>MODALITY_LABELS[input]).join(', ')}.` : ''} Output: ${task.outputs.map(output=>MODALITY_LABELS[output]).join(', ')}.</small></li>`;
  }).join('')}</ul></section>`;
}

/** @param {Array<{id:string}>} tasks @returns {string[]} */
export function defaultSelectedTasks(tasks) {
  if (!tasks.length) return [];
  return [(tasks.find(task=>task.id==='text') || tasks[0]).id];
}

/** @param {string[]} selected @param {Array<{id:string}>} tasks @returns {string[]} */
export function reconcileSelectedTasks(selected, tasks) {
  const available = new Set(tasks.map(task=>task.id));
  return selected.filter(id=>available.has(id));
}

/** @param {string[]} selected @param {string} taskID @param {Array<{id:string}>} tasks @returns {string[]} */
export function toggleSelectedTask(selected, taskID, tasks) {
  if (!tasks.some(task=>task.id===taskID)) return selected;
  if (selected.includes(taskID)) {
    return selected.filter(id=>id!==taskID);
  }
  return [...selected, taskID];
}

/** @param {{capabilities:string[]}} offering @param {string[]} selectedIDs @returns {boolean} */
export function offeringMatchesTasks(offering, selectedIDs) {
  if (!selectedIDs.length) return true;
  const supported = new Set(tasksForOffering(offering).map(task=>task.id));
  return selectedIDs.every(id=>supported.has(id));
}

/** @param {{capabilities:string[]}} offering @param {string[]} selectedIDs @param {string} inputModality @param {string} outputModality @returns {boolean} */
export function offeringMatchesTasksWithModalities(offering, selectedIDs, inputModality, outputModality) {
  if (!offeringMatchesTasks(offering, selectedIDs)) return false;
  if (!inputModality && !outputModality) return true;
  const offeringTasks = tasksForOffering(offering);
  const candidates = selectedIDs.length ? offeringTasks.filter(task=>selectedIDs.includes(task.id)) : offeringTasks;
  return candidates.some(task=>(!inputModality || /** @type {string[]} */ (task.inputs).includes(/** @type {Modality} */ (inputModality))) && (!outputModality || /** @type {string[]} */ (task.outputs).includes(/** @type {Modality} */ (outputModality))));
}

/** @param {ModelTask[]} tasks @param {string[]} selectedIDs @param {boolean} disabled */
export function renderTaskPicker(tasks, selectedIDs, disabled) {
  if (!tasks.length) return `<div class="task-filter" data-task-filter role="group" aria-label="Model tasks"><p class="cw-empty">No tasks for this connection.</p></div>`;
  const selected = new Set(selectedIDs);
  return `<div class="task-filter" data-task-filter role="group" aria-label="Model tasks">${tasks.map(task=>`<button type="button" data-task="${task.id}" aria-pressed="${selected.has(task.id)}" aria-label="${task.label}: ${task.description}" title="${task.description}" ${disabled?'disabled':''}>${renderTaskSymbol(task)}</button>`).join('')}</div>`;
}
