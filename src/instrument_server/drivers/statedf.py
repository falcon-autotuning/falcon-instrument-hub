"""The StateDF object is a data structure that represents a snapshot of the state of a device."""

import pandas as pd

from ..datatypes import Gate, Ohmic
from ..utils import write_device_voltages
from .sweep_datatypes import Waveform1D


class StateDF:
    """The StateDF object is a data structure that represents a snapshot of the state of a device.

    It stores the current voltage values for all gates and ohmics in the device. It is initialized with the global_ohmics and global_gates
    lists, which contain the ohmic and gate objects defined in the configuration. The channel1Inputs and
    channel2Inputs parameters contain the waveform objects for the first and second channels of the device, respectively.
    The end_voltages parameter is a dictionary that contains the voltage values for each gate and ohmic at the end of the
    measurement. Finally, the df parameter is a pandas DataFrame that contains the current voltage values for each gate
    and ohmic.

    TODO This will be deprecated in the future as it is no longer needed, and the main function is the current
    voltage state of the device.

    The StateDF object has the following attributes:
    - global_ohmics: list[Ohmic] | None - A list of ohmic objects defined in the configuration
    - global_gates: list[Gate] | None - A list of gate objects defined in the configuration
    - channel1Inputs: waveform1D | None - A waveform object for the first channel of the device
    - channel2Inputs: waveform1D | None - A waveform object for the second channel of the device
    - end_voltages: dict[Gate | Ohmic, float] - A dictionary that contains the voltage values for each gate and ohmic at
                                                the end of the measurement
    - df: pd.DataFrame | None - A pandas DataFrame that contains the current voltage values for each gate and ohmic

    The StateDF object has the following methods:
    - __init__(self, global_ohmics: list[Ohmic] | None = None, global_gates: list[Gate] | None = None,
               channel1Inputs: waveform1D | None = None, channel2Inputs: waveform1D | None = None,
               end_voltages: dict[Gate | Ohmic, float] = {}, df: pd.DataFrame | None = None) -> None
      - Initializes a StateDF object with the given parameters
    """

    df: pd.DataFrame

    def __init__(
        self,
        global_ohmics: list[Ohmic] | None = None,
        global_gates: list[Gate] | None = None,
        channel1Inputs: Waveform1D | None = None,
        channel2Inputs: Waveform1D | None = None,
        end_voltages: dict[Gate | Ohmic, float] = {},
        df: pd.DataFrame | None = None,
    ):
        """Function to generate a dataframe of metadata for all the gate voltages and sweeping informnation for each sweep.

        Args:
            global_ohmics (list[Ohmic]): List of ohmic devices
            global_gates (list[Gate]): List of gate devices
            channel1Inputs (waveform1D | None, optional): Input waveform for channel 1. Defaults to None.
            channel2Inputs (waveform1D | None, optional): Input waveform for channel 2. Defaults to None.
            end_voltages (dict[Gate | Ohmic, float], optional): Dictionary of gate voltages. Defaults to {}.
                This is specified for all connections
            df (pd.DataFrame | None, optional): Input dataframe. Defaults to None.

        The slow sweep in a 2D sweep is typically channel2 don't change this
        """
        if df is not None:
            self.df = df
            return
        state_df = pd.DataFrame(
            columns=[
                "Pads",
                "Sweep Start->End (mV)",
                "Functions",
                "Slow Sweep",
                "Favorite1",
                "Favorite2",
            ]
        )
        if global_ohmics is None:
            raise ValueError("No ohmics in the system")
        if global_gates is None:
            raise ValueError("No gates in the system")
        state_df["Pads"] = [pad.value for pad in global_ohmics + global_gates]
        # Default value for favorites column is blank string
        state_df["Favorite1"] = len(global_gates + global_ohmics) * [""]
        state_df["Favorite2"] = len(global_gates + global_ohmics) * [""]

        # Pulling information out of dictionary for both channel1Inputs and channel2Inputs
        if channel1Inputs is not None:
            for pad in list(channel1Inputs.keys()):
                # Storing dependancy data inside of the dataframe in the Favorite1 column
                idx = state_df.index[state_df["Pads"] == pad.value][0]
                state_df.at[idx, "Favorite1"] = "dep"
            # Storing favorite data inside of the dataframe in the Favorite1 column (overwritting dependancy)
            idx = state_df.index[
                state_df["Pads"] == channel1Inputs.get_favorite().value
            ][0]
            state_df.at[idx, "Favorite1"] = "fav"
        if channel2Inputs is not None:
            for pad in list(channel2Inputs.keys()):
                # Storing dependancy data inside of the dataframe in the Favorite1 column
                idx = state_df.index[state_df["Pads"] == pad.value][0]
                state_df.at[idx, "Favorite2"] = "dep"
            # Storing favorite data inside of the dataframe in the Favorite2 column (overwritting dependancy)
            idx = state_df.index[
                state_df["Pads"] == channel2Inputs.get_favorite().value
            ][0]
            state_df.at[idx, "Favorite2"] = "fav"

        for pad in global_gates + global_ohmics:
            # Store what each gate's voltage is
            v = end_voltages[pad]
            idx = state_df.index[state_df["Pads"] == pad.value][0]
            # Store the range the gates are being swept from with the format Start -> End. Default units is mV.
            if channel1Inputs is not None:
                favorite_pad1 = channel1Inputs.get_favorite()
                if pad in list(channel1Inputs.keys()):
                    state_df.at[idx, "Sweep Start->End (mV)"] = (
                        str(round(channel1Inputs[pad].v_start * 1e3, 3)) + "->" + str(v)
                    )
                    state_df.at[idx, "Functions"] = (
                        str(
                            round(
                                channel1Inputs[pad].amplitude
                                / channel1Inputs[favorite_pad1].amplitude,
                                3,
                            )
                        )
                        + "("
                        + str(channel1Inputs[favorite_pad1].name)
                        + ") + "
                        + str(
                            round(
                                channel1Inputs[pad].v_start
                                - channel1Inputs[favorite_pad1].v_start,
                                3,
                            )
                        )
                    )
                    state_df.at[idx, "Slow Sweep"] = "False"
            if channel2Inputs is not None:
                favorite_pad2 = channel2Inputs.get_favorite()
                if pad in list(channel2Inputs.keys()):
                    state_df.at[idx, "Sweep Start->End (mV)"] = (
                        str(round(channel2Inputs[pad].v_start * 1e3, 3)) + "->" + str(v)
                    )
                    state_df.at[idx, "Functions"] = (
                        str(
                            round(
                                channel2Inputs[pad].amplitude
                                / channel2Inputs[favorite_pad2].amplitude,
                                3,
                            )
                        )
                        + "("
                        + str(channel2Inputs[favorite_pad2].name)
                        + ") + "
                        + str(
                            round(
                                channel2Inputs[pad].v_start
                                - channel2Inputs[favorite_pad2].v_start,
                                3,
                            )
                        )
                    )
                    state_df.at[idx, "Slow Sweep"] = "True"
            if channel1Inputs is not None and channel2Inputs is not None:
                if pad not in list(channel2Inputs.keys()) and pad not in list(
                    channel1Inputs.keys()
                ):
                    state_df.at[idx, "Sweep Start->End (mV)"] = str(v)
                    state_df.at[idx, "Functions"] = ""
                    state_df.at[idx, "Slow Sweep"] = "False"
            if channel1Inputs is None and channel2Inputs is not None:
                if pad not in list(channel2Inputs.keys()):
                    state_df.at[idx, "Sweep Start->End (mV)"] = str(v)
                    state_df.at[idx, "Functions"] = ""
                    state_df.at[idx, "Slow Sweep"] = "False"
            if channel1Inputs is not None and channel2Inputs is None:
                if pad not in list(channel1Inputs.keys()):
                    state_df.at[idx, "Sweep Start->End (mV)"] = str(v)
                    state_df.at[idx, "Functions"] = ""
                    state_df.at[idx, "Slow Sweep"] = "False"
            if channel1Inputs is None and channel2Inputs is None:
                state_df.at[idx, "Sweep Start->End (mV)"] = str(v)
                state_df.at[idx, "Functions"] = ""
                state_df.at[idx, "Slow Sweep"] = "False"
        # changing pads to be string referenced
        for pad in global_gates + global_ohmics:
            idx = state_df.index[state_df["Pads"] == pad.value][0]
            state_df.at[idx, "Pads"] = pad.value
        self.df = state_df
        write_device_voltages(input=end_voltages)

    def _get_voltage(
        self,
        gate_name: Gate | Ohmic,
    ) -> float:
        """Returns the voltage of the specified gate.

        Args:
            gate_name (str): The name of the gate.

        Returns:
            float: The voltage of the gate.
        """
        voltage_series = self.df.loc[
            self.df["Pads"] == gate_name.value, "Sweep Start->End (mV)"
        ]
        voltage = voltage_series.iat[0]

        if ">" in voltage:
            voltage = float(voltage.split(">")[1])
        else:
            voltage = float(voltage)

        return voltage

    def get_gate_voltage(self, gate: Gate) -> float:
        """Returns the voltage of the specified gate.

        Args:
            gate (str): The name of the gate.

        Returns:
            float: The voltage of the gate. [mV]
        """
        return self._get_voltage(gate_name=gate)

    def get_ohmic_voltage(self, ohmic: Ohmic) -> float:
        """Returns the voltage of the specified ohmic.

        Args:
            ohmic (str): The name of the ohmic.

        Returns:
            float: The voltage of the ohmic.
        """
        return self._get_voltage(gate_name=ohmic)

    def get_gates_voltage(self, gates: list[Gate]) -> list[float]:
        """Returns the voltage of the specified gates.

        Args:
            gates (list[str]): The names of the gates.

        Returns:
            list[float]: The voltages of the gates.
        """
        return [self._get_voltage(gate_name=gate) for gate in gates]

    def get_favorite1(self) -> str:
        """Returns the name of the first favorite gate."""
        return list(self.df[self.df["Favorite1"] == "fav"]["Pads"])[0]

    def get_favorite2(self) -> str:
        """Returns the name of the second favorite gate."""
        return list(self.df[self.df["Favorite2"] == "fav"]["Pads"])[0]

    def get_deps1(self) -> list[str]:
        """Returns the names of the first dependent gates."""
        return list(self.df[self.df["Favorite1"] == "dep"]["Pads"])

    def get_deps2(self) -> list[str]:
        """Returns the names of the second dependent gates."""
        return list(self.df[self.df["Favorite2"] == "dep"]["Pads"])

    def get_sweep_start_end(
        self,
        pad: str,
    ) -> tuple[float, float]:
        """Returns the sweep start and end.

        Args:
            pad (str): The name of the pad.

        Returns:
            tuple[float, float]: The sweep start and end.
        """
        sweep = list(self.df[self.df["Pads"] == pad]["Sweep Start->End (mV)"])[0]
        start = float(sweep[: sweep.find("->")])
        stop = float(sweep[sweep.find(">") + 1 :])
        return (start, stop)

    def __repr__(self) -> str:
        """Returns the string representation of the StateDF object."""
        return str(self.__class__.__name__) + "(" + str(self.df) + ")"
