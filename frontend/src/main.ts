import {Events} from "@wailsio/runtime";
import {Service as GreetService} from "../bindings/github.com/petervdpas/formidable2/internal/services/greet";

const greetButton = document.getElementById('greet')! as HTMLButtonElement;
const resultElement = document.getElementById('result')! as HTMLDivElement;
const nameElement : HTMLInputElement = document.getElementById('name')! as HTMLInputElement;
const timeElement = document.getElementById('time')! as HTMLDivElement;

greetButton.addEventListener('click', async () => {
    let name = (nameElement as HTMLInputElement).value
    if (!name) {
        name = 'anonymous';
    }
    try {
        resultElement.innerText = await GreetService.Greet(name);
    } catch (err) {
        console.error(err);
    }
});

Events.On('time', (time) => {
    timeElement.innerText = time.data;
});
