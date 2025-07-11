var w=`<style>
  :host {
    --inspector-bg: #ebede9;
    --inspector-bg-light: #c7cfcc;
    --inspector-bg-hover: #d9dedc;
    --inspector-bg-dark: #a8b5b2;
    --inspector-color: #090a14;
    --inspector-color-blur: #819796;
    --inspector-color-filtered: #d73a49;
    --inspector-color-matched: #28a745;
    --color-purple: #A599FF;
    --color-gold: #FFA116;
    --color-html5: #E34C26;
    --color-html5-dark: #F06529;
    --color-dark-text: #0A0A0A;
    --color-white: #FFFFFF;
    background-color: var(--inspector-bg);
    border: 1px solid var(--inspector-bg-light);
    border-bottom: 0;
    border-radius: 7px 7px 0 0;
    color: var(--inspector-color);
    position: fixed;
    bottom: 0;
    right: 24px;
    max-width: 640px;
    margin: 0 5%;
    font-family: monospace;
    z-index: 9999;

    header, #collapsed {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 20px;
      padding: 8px 10px;
      font-size: 13px;
      font-weight: bold;
      user-select: none;
      cursor: pointer;
      box-sizing: border-box;
      height: 40px;

      button {
        padding: 2px 4px;
      }

      h1 {
        display: flex;
        align-items: center;
        gap: 8px;
        font-size: inherit;
        margin: 0;
        margin-left: -3px;

        button {
          font-size: 17px;
          padding: 0px 4px;
        }
      }
    }

    header {
      width: 640px;

      nav {
        display: flex;
        gap: 4px;
        
        button {
          display: flex;
          align-items: center;
          justify-content: center;
          gap: 4px;

          span {
            font-weight: bold;
          }
          
          svg {
            color: inherit;
          }
          
          &[data-type="currentSignals"][aria-selected="true"] {
            background-color: var(--color-purple);
            color: var(--color-dark-text);
            border-color: var(--color-purple);
          }
          
          &[data-type="signalPatchEvent"][aria-selected="true"] {
            background-color: var(--color-gold);
            color: var(--color-dark-text);
            border-color: var(--color-gold);
          }
          
          &[data-type="sseEvent"][aria-selected="true"] {
            background-color: var(--color-html5);
            color: var(--color-white);
            border-color: var(--color-html5);
          }
        }
      }
    }

    #collapsed {
      padding: 8px 12px 8px 15px;
    }

    button {
      background-color: var(--inspector-bg);
      border: 1px solid var(--inspector-bg-dark);
      border-radius: 4px;
      color: var(--inspector-color);
      font-family: inherit;
      font-size: 11px;
      flex-shrink: 0;
      cursor: pointer;

      &:hover {
        background-color: var(--inspector-bg-hover);
      }
    }

    pre {
      margin: 0;
    }

    main {
      max-height: 40vh;
      overflow-y: auto;
      font-size: 12px;
      border-top: 1px solid var(--inspector-bg-dark);

      .filters input[type="text"] {
        background-color: var(--inspector-bg);
        border: 1px solid var(--inspector-bg-dark);
        border-radius: 4px;
        color: var(--inspector-color);
        font-family: inherit;
        font-size: 11px;
        padding: 4px 6px;
        flex: 1;
        min-width: 0;
      }
      
      .input-with-icon {
        position: relative;
        display: flex;
        flex: 1;
        min-width: 0;
        
        input {
          padding-left: 32px;
          width: 100%;
        }
        
        .input-icon {
          position: absolute;
          left: 8px;
          top: 50%;
          transform: translateY(-50%);
          width: 16px;
          height: 16px;
          pointer-events: none;
          opacity: 0.6;
          color: var(--inspector-color);
        }
      }

      .filters label {
        display: flex;
        align-items: center;
        gap: 6px;
        font-size: 11px;
        user-select: none;
        cursor: pointer;
        white-space: nowrap;
        margin: 0 4px;
        
        span {
          font-weight: bold;
          font-family: monospace;
        }
      }

      .filters input[type="checkbox"] {
        cursor: pointer;
        margin: 0;
        flex-shrink: 0;
      }
        
      #resetFilters {
        padding-bottom: 0;
      }

      #copyFilterObject {
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 4px;
        flex-shrink: 0;
        gap: 4px;
        
        svg {
          width: 14px;
          height: 14px;
        }
      }

      .regex-display {
        display: flex;
        align-items: center;
        gap: 8px;
        width: 100%;
        font-size: 11px;
        font-family: monospace;
        
        .regex-content {
          display: flex;
          gap: 4px;
          flex: 1;
          
          &::before {
            content: "{";
            color: var(--inspector-color);
          }
          
          &::after {
            content: "}";
            color: var(--inspector-color);
          }
        }
        
        .regex-include {
          color: var(--inspector-color-matched);
        }
        
        .regex-exclude {
          color: var(--inspector-color-filtered);
        }
        
        .regex-include:empty,
        .regex-exclude:empty {
          display: none;
        }
        
        .regex-include:not(:empty)::before {
          content: "include: ";
          color: var(--inspector-color);
          font-weight: normal;
        }
        
        .regex-exclude:not(:empty)::before {
          content: ", exclude: ";
          color: var(--inspector-color);
          font-weight: normal;
        }
      }

      p {
        margin: 8px 0;
        padding: 4px 10px;
      }

      footer {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 8px 10px;
        border-top: 1px solid var(--inspector-bg-dark);
        background-color: var(--inspector-bg-hover);
        font-size: 11px;
        
        &.filters {
          flex-wrap: wrap;
          row-gap: 12px;
          column-gap: 8px;
        }
        
        label {
          font-weight: normal;
        }
      }

      ul {
        margin: 8px 0;
        padding: 0;
        list-style: none;

        li {
          display: flex;
          align-items: flex-start;
          gap: 4px;
          padding: 4px 10px;
          word-break: break-word;

          &:hover {
            background-color: var(--inspector-bg-hover);
          }

          span {
            flex-shrink: 0;
            margin-right: 4px;
          }

          div {
            display: flex;
            flex: 1;
            overflow-x: auto;
          }
          
          summary {
            list-style: none;
            position: relative;
            padding-right: 1.2em;
            cursor: pointer;
            width: fit-content;
            
            &::after {
              content: '\u25B8';
              position: absolute;
              top: 0;
              right: 0;
              font-size: 16px;
              line-height: 16px;
              transition: transform 0.2s;
            }
          }

          details[open] {
            summary.blurrable {
              color: var(--inspector-color-blur);
            }
            
            summary::after {
              transform: rotate(90deg);
            }
          }
        }
      }

      .currentSignals ul li {
        border-left: 3px solid var(--color-purple);

        &:hover {
          background-color: initial;
        }
      }

      .signalPatchEvents ul li {
        border-left: 3px solid var(--color-gold);
      }

      .sseEvents ul li {
        border-left: 3px solid var(--color-html5-dark);
      }

      button {
        padding: 2px 4px;
        flex-shrink: 0;
        
        &.copy-btn, &.log-btn {
          display: flex;
          align-items: center;
          justify-content: center;
          gap: 4px;
          
          svg {
            width: 16px;
            height: 16px;
          }
        }
      }
      
      input[type="number"] {
        background-color: var(--inspector-bg);
        border: 1px solid var(--inspector-bg-dark);
        border-radius: 4px;
        color: var(--inspector-color);
        font-family: inherit;
        font-size: 11px;
        padding: 4px 6px;
        flex-shrink: 0;
      }
      
      pre {
        .filtered {
          color: var(--inspector-color-filtered);
        }
        .matched {
          color: var(--inspector-color-matched);
        }
      }
      
      .signals-table {
        width: 100%;
        border-collapse: collapse;
        font-family: monospace;
        margin: 8px 0;
        
        th {
          background-color: var(--inspector-bg-dark);
          padding: 6px 10px;
          text-align: left;
          border: 1px solid var(--inspector-bg-light);
          font-weight: bold;
        }
        
        td {
          padding: 4px 10px;
          border: 1px solid var(--inspector-bg-light);
        }
        
        tr:nth-child(even) {
          background-color: var(--inspector-bg-hover);
        }
        
        tr.matched td {
          color: var(--inspector-color-matched);
        }
        
        tr.filtered td {
          color: var(--inspector-color-filtered);
        }
      }
      
      .toggle-switch {
        position: relative;
        display: inline-block;
        width: 40px;
        height: 20px;
        
        input {
          opacity: 0;
          width: 0;
          height: 0;
          
          &:checked + .toggle-slider {
            background-color: var(--inspector-color);
          }
          
          &:checked + .toggle-slider:before {
            transform: translateX(18px);
          }
        }
        
        .toggle-slider {
          position: absolute;
          cursor: pointer;
          top: 0;
          left: 0;
          right: 0;
          bottom: 0;
          background-color: var(--inspector-bg-dark);
          transition: .2s;
          border-radius: 20px;
          
          &:before {
            position: absolute;
            content: "";
            height: 16px;
            width: 16px;
            left: 2px;
            bottom: 2px;
            background-color: var(--inspector-bg);
            transition: .2s;
            border-radius: 50%;
          }
        }
      }
      
      .filter-row {
        display: flex;
        align-items: center;
        gap: 8px;
        width: 100%;
      }
      
      .filter-row-secondary {
        display: flex;
        align-items: center;
        justify-content: space-between;
        width: 100%;
      }
    }
  }


  @media (prefers-color-scheme: dark) {
    :host {
      --inspector-bg: #202e37;
      --inspector-bg-light: #394a50;
      --inspector-bg-hover: #1a262e;
      --inspector-bg-dark: #577277;
      --inspector-color: #ebede9;
      --inspector-color-blur: #819796;
      --inspector-color-filtered: #f97583;
      --inspector-color-matched: #34d058;
    }

    header nav button {
      &[data-type="currentSignals"][aria-selected="true"] {
        background-color: var(--color-purple);
        color: var(--color-dark-text);
        border-color: var(--color-purple);
      }
      
      &[data-type="signalPatchEvent"][aria-selected="true"] {
        background-color: var(--color-gold);
        color: var(--color-dark-text);
        border-color: var(--color-gold);
      }
      
      &[data-type="sseEvent"][aria-selected="true"] {
        background-color: var(--color-html5);
        color: var(--color-white);
        border-color: var(--color-html5);
      }
    }
  }
</style>

<template id="copy-button-template">
  <button class="copy-btn" title="Copy to clipboard">
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
      <path fill="currentColor" d="M4 2h11v2H6v13H4zm4 4h12v16H8zm2 2v12h8V8z"/>
    </svg>
    <span style="display: none"></span>
  </button>
</template>

<template id="log-button-template">
  <button class="log-btn" title="Log to the console">
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
      <path fill="currentColor" fill-rule="evenodd" d="M9.586 11L7.05 8.464L8.464 7.05l4.95 4.95l-4.95 4.95l-1.414-1.414L9.586 13H3v-2zM11 3h8c1.1 0 2 .9 2 2v14c0 1.1-.9 2-2 2h-8v-2h8V5h-8z"/>
    </svg>
    <span style="display: none"></span>
  </button>
</template>

<svg style="display: none;">
  <defs>
    <g id="datastar-logo">
      <g fill="#752438"><path d="m20 5h1v2h-1z"/><path d="m19 6h1v2h-1z"/><path d="m21 3h1v2h-1z"/><path d="m5 13h1v1h-1z"/><path d="m10 19h1v1h-1z"/><path d="m14 15h1v1h-1z"/><path d="m13 16h1v1h-1z"/><path d="m12 17h1v1h-1z"/><path d="m4 14h1v1h-1z"/><path d="m17 6h1v1h-1z"/></g>
      <g fill="#73bed3"><path d="m11 7h1v1h-1z"/><path d="m12 6h1v1h-1z"/><path d="m13 5h1v1h-1z"/><path d="m9 9h1v1h-1z"/><path d="m10 8h1v1h-1z"/></g>
      <g fill="#172038"><path d="m9 12h1v1h-1z"/><path d="m16 12h1v1h-1z"/><path d="m12 15h1v2h-1z"/><path d="m15 13h1v1h-1z"/><path d="m13 15h1v1h-1z"/><path d="m8 11h1v1h-1z"/><path d="m17 11h1v1h-1z"/><path d="m10 17h2v1h-2z"/><path d="m6 14h1v1h-1z"/><path d="m11 14h1v1h-1z"/><path d="m14 14h1v1h-1z"/><path d="m18 10h1v1h-1z"/></g>
      <g fill="#be772b"><path d="m8 19h1v2h-1z"/><path d="m5 20h2v1h-2z"/><path d="m9 18h1v1h-1z"/><path d="m5 16h1v1h-1z"/></g>
      <g fill="#da863e"><path d="m17 2h3v1h-3z"/><path d="m15 4h1v1h-1z"/><path d="m16 3h1v1h-1z"/></g>
      <g fill="#cf573c"><path d="m6 10h1v1h-1z"/><path d="m4 12h1v1h-1z"/><path d="m10 20h1v1h-1z"/><path d="m3 13h1v1h-1z"/><path d="m9 13h1v1h-1z"/><path d="m20 2h2v1h-2z"/><path d="m11 19h1v1h-1z"/><path d="m6 16h1v1h-1z"/><path d="m14 16h1v1h-1z"/><path d="m7 15h1v1h-1z"/><path d="m13 17h1v1h-1z"/><path d="m18 3h1v1h-1z"/><path d="m5 11h1v1h-1z"/><path d="m2 14h1v1h-1z"/><path d="m8 14h1v1h-1z"/><path d="m17 3h1v2h-1z"/><path d="m12 18h1v1h-1z"/></g>
      <g fill="#3c5e8b"><path d="m11 9h1v3h-1z"/><path d="m7 14h1v1h-1z"/><path d="m15 11h1v1h-1z"/><path d="m11 15h1v1h-1z"/><path d="m14 10h1v3h-1z"/><path d="m12 8h1v5h-1z"/><path d="m8 12h1v2h-1z"/><path d="m13 7h1v1h-1z"/><path d="m16 7h1v4h-1z"/><path d="m6 13h1v1h-1z"/><path d="m15 6h1v2h-1z"/><path d="m13 11h1v3h-1z"/><path d="m14 6h1v3h-1z"/><path d="m13 9h1v1h-1z"/><path d="m15 9h1v1h-1z"/><path d="m17 8h1v2h-1z"/><path d="m10 15h1v2h-1z"/><path d="m8 10h1v1h-1z"/><path d="m10 10h1v1h-1z"/></g>
      <g fill="#253a5e"><path d="m17 10h1v1h-1z"/><path d="m11 13h1v1h-1z"/><path d="m14 13h1v1h-1z"/><path d="m18 9h1v1h-1z"/><path d="m9 16h1v1h-1z"/><path d="m11 16h1v1h-1z"/><path d="m7 12h1v2h-1z"/><path d="m12 14h1v1h-1z"/><path d="m9 11h1v1h-1z"/><path d="m16 11h1v1h-1z"/><path d="m10 12h1v1h-1z"/><path d="m15 12h1v1h-1z"/></g>
      <g fill="#e8c170"><path d="m8 17h1v1h-1z"/><path d="m6 15h1v1h-1z"/><path d="m4 19h1v2h-1z"/><path d="m5 18h1v2h-1z"/><path d="m3 20h1v2h-1z"/><path d="m7 18h1v1h-1z"/></g>
      <g fill="#241527"><path d="m20 7h1v1h-1z"/><path d="m18 8h1v1h-1z"/><path d="m19 8h1v2h-1z"/><path d="m21 5h1v2h-1z"/></g>
      <g fill="#602c2c"><path d="m9 17h1v1h-1z"/><path d="m6 18h1v1h-1z"/></g>
      <g fill="#4f8fba"><path d="m11 8h1v1h-1z"/><path d="m7 11h1v1h-1z"/><path d="m10 11h1v1h-1z"/><path d="m13 6h1v1h-1z"/><path d="m13 14h1v1h-1z"/><path d="m9 10h1v1h-1z"/><path d="m6 12h1v1h-1z"/><path d="m11 12h1v1h-1z"/><path d="m14 5h1v1h-1z"/><path d="m10 9h1v1h-1z"/><path d="m12 13h1v1h-1z"/><path d="m12 7h1v1h-1z"/></g>
      <g fill="#a53030"><path d="m19 3h1v3h-1z"/><path d="m6 11h1v1h-1z"/><path d="m6 17h1v1h-1z"/><path d="m14 17h1v1h-1z"/><path d="m13 18h1v1h-1z"/><path d="m20 3h1v2h-1z"/><path d="m3 14h1v1h-1z"/><path d="m9 14h1v1h-1z"/><path d="m12 19h1v1h-1z"/><path d="m8 15h1v1h-1z"/><path d="m7 16h1v1h-1z"/><path d="m4 13h1v1h-1z"/><path d="m10 13h1v1h-1z"/><path d="m18 4h1v3h-1z"/><path d="m7 10h1v1h-1z"/><path d="m16 4h1v2h-1z"/><path d="m11 20h1v1h-1z"/><path d="m17 5h1v1h-1z"/><path d="m5 12h1v1h-1z"/><path d="m10 21h1v1h-1z"/><path d="m14 4h1v1h-1z"/></g>
      <g fill="#411d31"><path d="m15 5h1v1h-1z"/><path d="m8 16h1v1h-1z"/><path d="m9 15h1v1h-1z"/><path d="m17 7h2v1h-2z"/><path d="m16 6h1v1h-1z"/><path d="m5 14h1v1h-1z"/><path d="m10 14h1v1h-1z"/><path d="m10 18h2v1h-2z"/><path d="m7 17h1v1h-1z"/></g>
      <g fill="#a4dddb"><path d="m13 10h1v1h-1z"/><path d="m15 10h1v1h-1z"/><path d="m13 8h1v1h-1z"/><path d="m15 8h1v1h-1z"/><path d="m14 9h1v1h-1z"/></g>
      <g fill="#de9e41"><path d="m4 18h1v1h-1z"/><path d="m8 18h1v1h-1z"/><path d="m5 17h1v1h-1z"/><path d="m3 19h1v1h-1z"/><path d="m6 19h2v1h-2z"/></g>
    </g>
  </defs>
</svg>

<div id="collapsed">
  <h1>
    <svg viewBox="0 0 24 24" width="24" height="24" xmlns="http://www.w3.org/2000/svg">
      <use href="#datastar-logo"/>
    </svg>
  </h1>
</div>
<header style="display: none">
  <h1>
    <button aria-label="Collapse">\xBB</button>
    Datastar Inspector
    <svg viewBox="0 0 24 24" width="20" height="20" xmlns="http://www.w3.org/2000/svg">
      <use href="#datastar-logo"/>
    </svg>
  </h1>
  <nav>
    <button data-type="currentSignals" aria-selected="true" aria-label="Switch to current signals tab" title="Current signals with the ability to filter">
      <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 16 16"><path fill="currentColor" fill-rule="evenodd" d="M12.442 13.033c-.278.307-.319.777-.05 1.092c.27.314.747.353 1.033.053a7.5 7.5 0 1 0-10.85 0c.286.3.763.261 1.032-.053c.27-.315.23-.785-.05-1.092a6 6 0 1 1 8.884 0m-.987-1.15c-.265.318-.745.279-1.015-.036c-.27-.314-.223-.784.015-1.123a3 3 0 1 0-4.91 0c.238.339.284.809.015 1.123c-.27.315-.75.354-1.015.036a4.5 4.5 0 1 1 6.91 0M8 10.5a1.5 1.5 0 1 0 0-3a1.5 1.5 0 0 0 0 3" clip-rule="evenodd"/></svg>
      <span id="currentSignalsCount">0</span>
    </button>
    <button data-type="signalPatchEvent" aria-selected="false" aria-label="Switch to signal patch events tab" title="Signal patch events">
      <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path fill="currentColor" d="M5 3h2v2H5v5a2 2 0 0 1-2 2a2 2 0 0 1 2 2v5h2v2H5c-1.07-.27-2-.9-2-2v-4a2 2 0 0 0-2-2H0v-2h1a2 2 0 0 0 2-2V5a2 2 0 0 1 2-2m14 0a2 2 0 0 1 2 2v4a2 2 0 0 0 2 2h1v2h-1a2 2 0 0 0-2 2v4a2 2 0 0 1-2 2h-2v-2h2v-5a2 2 0 0 1 2-2a2 2 0 0 1-2-2V5h-2V3zm-7 12a1 1 0 0 1 1 1a1 1 0 0 1-1 1a1 1 0 0 1-1-1a1 1 0 0 1 1-1m-4 0a1 1 0 0 1 1 1a1 1 0 0 1-1 1a1 1 0 0 1-1-1a1 1 0 0 1 1-1m8 0a1 1 0 0 1 1 1a1 1 0 0 1-1 1a1 1 0 0 1-1-1a1 1 0 0 1 1-1"/></svg>
      <span id="signalPatchEventCount">0</span>
    </button>
    <button data-type="sseEvent" aria-selected="false" aria-label="Switch to SSE tab" title="SSE events">
      <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path fill="currentColor" d="m12 17.56l4.07-1.13l.55-6.1H9.38L9.2 8.3h7.6l.2-1.99H7l.56 6.01h6.89l-.23 2.58l-2.22.6l-2.22-.6l-.14-1.66h-2l.29 3.19zM4.07 3h15.86L18.5 19.2L12 21l-6.5-1.8z"/></svg>
      <span id="sseEventCount">0</span>
    </button>
  </nav>
</header>
<main style="display: none">
  <section id="currentSignalsContent" class="currentSignals">
    <ul>
      <li>
        <pre></pre>
      </li>
    </ul>
    <footer class="filters">
      <div class="filter-row">
        <div class="input-with-icon">
          <input type="text" id="currentSignalsIncludeFilter" placeholder="Include filter (default: .*)" title="Only show signals matching this pattern. Use regex or wildcards (with Wildcard checked). Default '.*' matches all signals." />
        </div>
        <div class="input-with-icon">
          <input type="text" id="currentSignalsExcludeFilter" placeholder="Exclude filter (default: (^|\\.)_)" title="Hide signals matching this pattern. Use regex or wildcards (with Wildcard checked). Default '(^|\\.)_' hides signals starting with underscore or containing '._'." />
        </div>
        <button id="resetFilters" title="Reset filters">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24">
              <path fill="currentColor" d="M11.77 3c-2.65.07-5 1.28-6.6 3.16L3.85 4.85a.5.5 0 0 0-.85.36V9.5c0 .28.22.5.5.5h4.29c.45 0 .67-.54.35-.85L6.59 7.59C7.88 6.02 9.82 5 12 5c4.32 0 7.74 3.94 6.86 8.41c-.54 2.77-2.81 4.98-5.58 5.47c-3.8.68-7.18-1.74-8.05-5.16c-.12-.42-.52-.72-.96-.72c-.65 0-1.14.61-.98 1.23C4.28 18.12 7.8 21 12 21c5.06 0 9.14-4.17 9-9.26c-.14-4.88-4.35-8.86-9.23-8.74M14 12c0-1.1-.9-2-2-2s-2 .9-2 2s.9 2 2 2s2-.9 2-2" />
            </svg>
            <span style="display: none"></span>
          </button>
        <label title="Enable wildcard: * matches any character">
          <input type="checkbox" id="currentSignalsWildcardCheckbox" />
          <span>Wildcard</span>
        </label>
      </div>
      <div class="filter-row-secondary">
        <div class="regex-display" id="currentSignalsRegexDisplay" style="display: none;">
          <button id="copyFilterObject" title="Copy filter as regex object">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24">
              <path fill="currentColor" d="M4 2h11v2H6v13H4zm4 4h12v16H8zm2 2v12h8V8z"/>
            </svg>
            <span style="display: none"></span>
          </button>
          <span class="regex-content">
            <span class="regex-include"></span>
            <span class="regex-exclude"></span>
          </span>
        </div>
        <label title="Toggle between object and path view" style="display: flex; align-items: center; gap: 8px;">
          <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 256 256"><!-- Icon from Phosphor by Phosphor Icons - https://github.com/phosphor-icons/core/blob/main/LICENSE --><g fill="currentColor"><path d="M240 128c-64 0 0 88-64 88H80c-64 0 0-88-64-88c64 0 0-88 64-88h96c64 0 0 88 64 88" opacity=".2"/><path d="M43.18 128a29.8 29.8 0 0 1 8 10.26c4.8 9.9 4.8 22 4.8 33.74c0 24.31 1 36 24 36a8 8 0 0 1 0 16c-17.48 0-29.32-6.14-35.2-18.26c-4.8-9.9-4.8-22-4.8-33.74c0-24.31-1-36-24-36a8 8 0 0 1 0-16c23 0 24-11.69 24-36c0-11.72 0-23.84 4.8-33.74C50.68 38.14 62.52 32 80 32a8 8 0 0 1 0 16c-23 0-24 11.69-24 36c0 11.72 0 23.84-4.8 33.74A29.8 29.8 0 0 1 43.18 128M240 120c-23 0-24-11.69-24-36c0-11.72 0-23.84-4.8-33.74C205.32 38.14 193.48 32 176 32a8 8 0 0 0 0 16c23 0 24 11.69 24 36c0 11.72 0 23.84 4.8 33.74a29.8 29.8 0 0 0 8 10.26a29.8 29.8 0 0 0-8 10.26c-4.8 9.9-4.8 22-4.8 33.74c0 24.31-1 36-24 36a8 8 0 0 0 0 16c17.48 0 29.32-6.14 35.2-18.26c4.8-9.9 4.8-22 4.8-33.74c0-24.31 1-36 24-36a8 8 0 0 0 0-16"/></g></svg>
          <div class="toggle-switch">
            <input type="checkbox" id="currentSignalsTableViewCheckbox" />
            <span class="toggle-slider"></span>
          </div>
          <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><!-- Icon from Material Symbols by Google - https://github.com/google/material-design-icons/blob/master/LICENSE --><path fill="currentColor" d="M5 21q-.825 0-1.412-.587T3 19V5q0-.825.588-1.412T5 3h14q.825 0 1.413.588T21 5v14q0 .825-.587 1.413T19 21zm6-6H5v4h6zm2 0v4h6v-4zm-2-2V9H5v4zm2 0h6V9h-6zM5 7h14V5H5z"/></svg>
        </label>
      </div>
    </footer>
  </section>
  <section id="signalPatchEventContent" class="signalPatchEvents" style="display: none">
    <p>No events yet.</p>
    <ul></ul>
    <footer>
      <label for="signalPatchMaxEvents">Max events:</label>
      <input type="number" id="signalPatchMaxEvents" placeholder="Max events" min="1" style="width: 80px;" />
      <button data-type="signalPatchEvent" style="display: none">Clear</button>
    </footer>
  </section>
  <section id="sseEventContent" class="sseEvents" style="display: none">
    <p>No events yet.</p>
    <ul></ul>
    <footer>
      <label for="sseMaxEvents">Max events:</label>
      <input type="number" id="sseMaxEvents" placeholder="Max events" min="1" style="width: 80px;" />
      <button data-type="sseEvent" style="display: none">Clear</button>
    </footer>
  </section>
</main>`;var f=class extends HTMLElement{constructor(){super();this.expanded=!1;this.open=!1;this.tab="currentSignals";this.maxEventsVisible=20;this.signalPatchEventCount=0;this.sseEventCount=0;this.signalPatchEvents=void 0;this.sseEvents=void 0;this.currentSignalsIncludeFilter="";this.currentSignalsExcludeFilter="";this.currentSignalsUseWildcard=!1;this.currentSignalsTableView=!1;this.currentSignals={};this.attachShadow({mode:"open"});let t=sessionStorage.getItem("datastar-inspector-state");if(t){let e=JSON.parse(t);this.expanded=e.expanded??this.expanded,this.open=e.open??this.open,this.tab=e.tab??this.tab,this.currentSignalsIncludeFilter=e.currentSignalsIncludeFilter??this.currentSignalsIncludeFilter,this.currentSignalsExcludeFilter=e.currentSignalsExcludeFilter??this.currentSignalsExcludeFilter,this.currentSignalsUseWildcard=e.currentSignalsUseWildcard??this.currentSignalsUseWildcard,this.currentSignalsTableView=e.currentSignalsTableView??this.currentSignalsTableView}let s="el.setAttribute('data-current-signals', JSON.stringify($))";for(let e of["data-on-load","data-on-signal-patch"])this.setAttribute(e,s)}static get observedAttributes(){return["data-current-signals","max-events-visible"]}connectedCallback(){this.render(),document.addEventListener("datastar-sse",this.handleSseEvent.bind(this)),document.addEventListener("datastar-signal-patch",this.handleSignalPatchEvent.bind(this));let t=this.getAttribute("max-events-visible");if(t){let s=Number.parseInt(t);!Number.isNaN(s)&&s>0&&(this.maxEventsVisible=s)}}disconnectedCallback(){this.removeEventListener("datastar-sse",this.handleSseEvent),this.removeEventListener("datastar-signal-patch",this.handleSignalPatchEvent)}attributeChangedCallback(t,s,e){t==="data-current-signals"&&s!==e&&e!==null&&this.handleCurrentSignalsChange(e),t==="max-events-visible"&&s!==e&&e!==null&&(this.maxEventsVisible=Number.parseInt(e),this.garbageCollectEvents(this.shadowRoot?.querySelector("#sseEventContent ul")),this.garbageCollectEvents(this.shadowRoot?.querySelector("#signalPatchEventContent ul")))}render(){if(!this.shadowRoot)return;this.shadowRoot.innerHTML=w,this.renderExpand(),this.renderOpen(),this.renderTab();for(let e of this.shadowRoot.querySelectorAll("#collapsed, h1 button"))e.addEventListener("click",()=>this.toggleExpand());for(let e of this.shadowRoot.querySelectorAll("#collapsed, header"))e.addEventListener("click",()=>this.toggleOpen());for(let e of this.shadowRoot.querySelectorAll("header h1 button, header nav"))e.addEventListener("click",i=>i.stopPropagation());for(let e of this.shadowRoot.querySelectorAll("header nav button"))e.addEventListener("click",()=>this.selectTab(e.dataset.type||""));for(let e of this.shadowRoot.querySelectorAll("footer button[data-type]"))e.addEventListener("click",()=>this.clearEvents(e.dataset.type||""));let t=this.shadowRoot.querySelector("#signalPatchMaxEvents"),s=this.shadowRoot.querySelector("#sseMaxEvents");t&&(t.value=this.maxEventsVisible.toString(),t.addEventListener("input",()=>{let e=Number.parseInt(t.value);!Number.isNaN(e)&&e>0&&(this.maxEventsVisible=e,s&&(s.value=e.toString()),this.garbageCollectEvents(this.shadowRoot?.querySelector("#signalPatchEventContent ul")),this.garbageCollectEvents(this.shadowRoot?.querySelector("#sseEventContent ul")))})),s&&(s.value=this.maxEventsVisible.toString(),s.addEventListener("input",()=>{let e=Number.parseInt(s.value);!Number.isNaN(e)&&e>0&&(this.maxEventsVisible=e,t&&(t.value=e.toString()),this.garbageCollectEvents(this.shadowRoot?.querySelector("#signalPatchEventContent ul")),this.garbageCollectEvents(this.shadowRoot?.querySelector("#sseEventContent ul")))})),this.setupFilterHandlers()}setupFilterHandlers(){if(!this.shadowRoot)return;let t=this.shadowRoot.querySelector("#currentSignalsIncludeFilter"),s=this.shadowRoot.querySelector("#currentSignalsExcludeFilter"),e=this.shadowRoot.querySelector("#currentSignalsWildcardCheckbox"),i=this.shadowRoot.querySelector("#resetFilters"),n=this.shadowRoot.querySelector("#copyFilterObject"),a=this.shadowRoot.querySelector("#currentSignalsTableViewCheckbox");t&&(t.value=this.currentSignalsIncludeFilter||(this.currentSignalsUseWildcard?"":".*"),t.addEventListener("input",()=>{this.currentSignalsIncludeFilter=t.value,this.saveState(),this.updateCurrentSignalsDisplay(),this.updateRegexDisplay()})),s&&(s.value=this.currentSignalsExcludeFilter||(this.currentSignalsUseWildcard?"":"(^|\\.)_"),s.addEventListener("input",()=>{this.currentSignalsExcludeFilter=s.value,this.saveState(),this.updateCurrentSignalsDisplay(),this.updateRegexDisplay()})),e&&(e.checked=this.currentSignalsUseWildcard,e.addEventListener("change",()=>{this.currentSignalsUseWildcard=e.checked,this.saveState(),this.updateCurrentSignalsDisplay(),this.updateRegexDisplay()})),i&&i.addEventListener("click",()=>{this.currentSignalsIncludeFilter=".*",this.currentSignalsExcludeFilter="(^|\\.)_",this.currentSignalsUseWildcard=!1,t.value=this.currentSignalsIncludeFilter,s.value=this.currentSignalsExcludeFilter,e.checked=this.currentSignalsUseWildcard,this.saveState(),this.updateCurrentSignalsDisplay(),this.updateRegexDisplay()}),n&&n.addEventListener("click",()=>{this.copyFilterObject(n)}),a&&(a.checked=this.currentSignalsTableView,a.addEventListener("change",()=>{this.currentSignalsTableView=a.checked,this.saveState(),this.updateCurrentSignalsDisplay()})),this.updateRegexDisplay()}handleCurrentSignalsChange(t){this.currentSignals=JSON.parse(t),this.updateCurrentSignalsDisplay()}wildcardAsRegex(t){return RegExp(`^${t.replace(/\./g,"\\.").replace(/\*/g,".*")}$`)}wildcardToRegex(t){try{return this.wildcardAsRegex(t).toString().replace(/^\/\^/,"").replace(/\$\/$/,"")}catch{return t}}flattenObject(t,s=""){let e=[];for(let[i,n]of Object.entries(t)){let a=s?`${s}.${i}`:i;n!==null&&typeof n=="object"?Array.isArray(n)?n.forEach((o,h)=>{o!==null&&typeof o=="object"?e.push(...this.flattenObject(o,`${a}.${h}`)):e.push([`${a}.${h}`,o])}):e.push(...this.flattenObject(n,a)):e.push([a,n])}return e}updateRegexDisplay(){if(!this.shadowRoot)return;let t=this.shadowRoot.querySelector("#currentSignalsRegexDisplay"),s=t?.querySelector(".regex-include"),e=t?.querySelector(".regex-exclude");if(!t||!s||!e)return;t.style.display="";let i=this.currentSignalsIncludeFilter||".*",n=this.currentSignalsExcludeFilter||"(^|\\.)_";s.textContent=this.currentSignalsUseWildcard&&this.currentSignalsIncludeFilter?this.wildcardAsRegex(i).toString():`/${i}/`,e.textContent=this.currentSignalsUseWildcard&&this.currentSignalsExcludeFilter?this.wildcardAsRegex(n).toString():`/${n}/`}copyFilterObject(t){let s=this.currentSignalsIncludeFilter||".*",e=this.currentSignalsExcludeFilter||"(^|\\.)_",i={include:this.currentSignalsUseWildcard?this.wildcardAsRegex(s).toString():`/${s}/`,exclude:this.currentSignalsUseWildcard?this.wildcardAsRegex(e).toString():`/${e}/`},n=JSON.stringify(i,null,1),a=t.querySelector("span");navigator.clipboard.writeText(n).then(()=>{a.textContent="Copied",a.style.display=""}).catch(()=>{a.textContent="Copy failed",a.style.display=""}).finally(()=>{setTimeout(()=>{a.style.display="none"},2e3)})}updateCurrentSignalsDisplay(){this.currentSignalsTableView?this.updateCurrentSignalsTableDisplay():this.updateCurrentSignalsTreeDisplay()}updateCurrentSignalsTableDisplay(){let t=this.currentSignalsIncludeFilter||this.currentSignalsExcludeFilter,s=this.flattenObject(this.currentSignals),e=0,a=`
      <table class="signals-table">
        <thead>
          <tr>
            <th>Path</th>
            <th>Value</th>
          </tr>
        </thead>
        <tbody>
          ${s.map(([h,d])=>{let l=this.matchesFilter(h,this.currentSignalsIncludeFilter,this.currentSignalsExcludeFilter,this.currentSignalsUseWildcard);return l&&e++,{path:h,value:d,matches:l}}).map(({path:h,value:d,matches:l})=>{let r=JSON.stringify(d),p=t?l?"matched":"filtered":"";return`<tr${p?` class="${p}"`:""}><td>${this.escapeHtml(h)}</td><td>${this.escapeHtml(r)}</td></tr>`}).join(`
`)}
        </tbody>
      </table>
    `;this.setTextContent("#currentSignalsCount",t?`${e}/${s.length}`:s.length);let o=this.shadowRoot?.querySelector("#currentSignalsContent ul li");o&&(o.innerHTML=a)}escapeHtml(t){let s=document.createElement("div");return s.textContent=t,s.innerHTML}updateCurrentSignalsTreeDisplay(){let t=this.currentSignalsIncludeFilter||this.currentSignalsExcludeFilter,s=new Set,e=0;if(t){let h=this.flattenObject(this.currentSignals);for(let[d]of h)if(this.matchesFilter(d,this.currentSignalsIncludeFilter,this.currentSignalsExcludeFilter,this.currentSignalsUseWildcard)){e++;let l=d.split(".");for(let r=1;r<=l.length;r++)s.add(l.slice(0,r).join("."))}}let i=(h,d="",l="  ")=>{let r=[],p=Object.entries(h);return p.forEach(([c,b],y)=>{let x=d?`${d}.${c}`:c,u=y===p.length-1,g=s.has(x);if(b!==null&&typeof b=="object"&&!Array.isArray(b)){t?g?r.push(`${l}<span class="matched">"${c}": {`):r.push(`${l}<span class="filtered">"${c}": {`):r.push(`${l}"${c}": {`);let S=i(b,x,`${l}  `);r.push(...S),t?r.push(`${l}}${u?"</span>":",</span>"}`):r.push(`${l}}${u?"":","}`)}else{let v=JSON.stringify(b,null,2).split(`
`);if(t){g?r.push(`${l}<span class="matched">"${c}": ${v[0]}`):r.push(`${l}<span class="filtered">"${c}": ${v[0]}`);for(let m=1;m<v.length;m++)r.push(`${l}${v[m]}`);r[r.length-1]+=u?"</span>":",</span>"}else{r.push(`${l}"${c}": ${v[0]}`);for(let m=1;m<v.length;m++)r.push(`${l}${v[m]}`);u||(r[r.length-1]+=",")}}}),r},n=[];t&&e>0?n.push('<span class="matched">{</span>'):n.push("{"),n.push(...i(this.currentSignals)),t&&e>0?n.push('<span class="matched">}</span>'):n.push("}");let a=t?this.flattenObject(this.currentSignals).length:Object.keys(this.currentSignals).length;this.setTextContent("#currentSignalsCount",t?`${e}/${a}`:a);let o=this.shadowRoot?.querySelector("#currentSignalsContent ul li");o&&(o.innerHTML=`<pre>${n.join(`
`)}</pre>`),t&&e>0?n.push('<span class="matched">}</span>'):n.push("}")}handleSignalPatchEvent(t){let s=t.detail,e=JSON.stringify(s);this.signalPatchEventCount++,this.setTextContent("#signalPatchEventCount",this.signalPatchEventCount),this.appendEvent(this.signalPatchEventCount,"#signalPatchEventContent",e,JSON.stringify(s,null,2),s,!0)}handleSseEvent(t){let s=t.detail;if(!s.type.startsWith("datastar-"))return;this.sseEventCount++,this.setTextContent("#sseEventCount",this.sseEventCount);let e="",i=s.argsRaw;for(let[n,a]of Object.entries(i))e+=`
data: ${n}: ${a}`;this.appendEvent(this.sseEventCount,"#sseEventContent",`event: ${s.type}`,e,s,!1)}toggleExpand(){this.expanded=!this.expanded,this.renderExpand(),this.saveState()}renderExpand(){this.setVisibility("#collapsed",!this.expanded),this.setVisibility("header",this.expanded),this.expanded||(this.open=!0,this.toggleOpen())}toggleOpen(){this.open=!this.open,this.renderOpen(),this.saveState()}renderOpen(){this.setVisibility("main",this.open)}selectTab(t){this.open=!0,this.tab=t,this.renderTab(),this.saveState()}renderTab(){this.setAttributeValue("header button","aria-selected","false"),this.setAttributeValue(`header button[data-type=${this.tab}]`,"aria-selected","true"),this.setVisibility("main",this.open),this.setVisibility("main section",!1),this.setVisibility(`#${this.tab}Content`,!0),this.saveState()}saveState(){let t={expanded:this.expanded,open:this.open,tab:this.tab,currentSignalsIncludeFilter:this.currentSignalsIncludeFilter,currentSignalsExcludeFilter:this.currentSignalsExcludeFilter,currentSignalsUseWildcard:this.currentSignalsUseWildcard,currentSignalsTableView:this.currentSignalsTableView};sessionStorage.setItem("datastar-inspector-state",JSON.stringify(t))}clearEvents(t){t==="sse"?this.sseEventCount=0:this.signalPatchEventCount=0,this.setTextContent(`#${t}Count`,0),this.setInnerHtml(`#${t}Content ul`,""),this.setVisibility(`#${t}Content footer button[data-type]`,!1),this.setVisibility(`#${t}Content p`,!0)}setTextContent(t,s){let e=this.shadowRoot?.querySelector(t);e&&(e.textContent=`${s}`)}setInnerHtml(t,s){let e=this.shadowRoot?.querySelector(t);e&&(e.innerHTML=s)}setAttributeValue(t,s,e){for(let i of this.shadowRoot?.querySelectorAll(t)||[])i.setAttribute(s,e)}setVisibility(t,s){this.setAttributeValue(t,"style",s?"":"display: none;")}appendEvent(t,s,e,i,n,a){let o=this.shadowRoot?.querySelector(`${s} ul`);if(!o)return;let h=document.createElement("pre");h.textContent=i;let d=document.createElement("div");d.innerHTML=`
      <span>${t}.</span>
      <details>
        <summary class="${a?"blurrable":""}">${e}</summary>
        <pre>${h.innerHTML}</pre>
      </details>
    `;let p=(this.shadowRoot?.querySelector("#copy-button-template")).content.cloneNode(!0).querySelector("button"),c=p.querySelector("span");p.onclick=()=>{navigator.clipboard.writeText(i).then(()=>{c.textContent="Copied",c.style.display=""}).catch(()=>{c.textContent="Copy Failed",c.style.display=""}).finally(()=>{setTimeout(()=>{c.style.display="none"},2e3)})};let x=(this.shadowRoot?.querySelector("#log-button-template")).content.cloneNode(!0).querySelector("button"),u=x.querySelector("span");x.onclick=()=>{console.log(n),u.textContent="Logged",u.style.display="",setTimeout(()=>{u.style.display="none"},2e3)};let g=document.createElement("li");g.appendChild(d),g.appendChild(p),g.appendChild(x),o.prepend(g),this.garbageCollectEvents(o),this.setVisibility(`${s} footer button[data-type]`,!0),this.setVisibility(`${s} p`,!1)}createButton(t,s){let e=document.createElement("button");return e.textContent=t,e.onclick=s,e}garbageCollectEvents(t){if(t)for(;t.children.length>this.maxEventsVisible;)t.removeChild(t.lastChild)}matchesFilter(t,s,e,i){let n=s||".*",a=e||"(^|\\.)_";if(i){let o=this.wildcardAsRegex(n),h=this.wildcardAsRegex(a);return o.test(t)&&!h.test(t)}try{let o=new RegExp(n),h=new RegExp(a);return o.test(t)&&!h.test(t)}catch{return t.includes(n)&&!t.includes(a)}}};customElements.define("datastar-inspector",f);
//# sourceMappingURL=datastar-inspector.js.map