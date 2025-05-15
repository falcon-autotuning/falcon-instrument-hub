# Eriksson group QCoDeS driver for the NiDFAQ6216 using ai channels

import logging

import numpy as np
import pandas as pd
from nidaqmx.constants import Edge
from qcodes import validators as vals
from qcodes.instrument import ChannelList, Instrument, InstrumentChannel
from qcodes.parameters import Parameter, ParameterWithSetpoints

LOG = logging.getLogger(__name__)


class GeneratedTimeArray(Parameter):
    """A parameter that generates a time array from sample rate and number of points
    parameters.
    """

    def __init__(self, samplerate, n_points, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.samplerate = samplerate
        self.n_points = n_points

    def get_raw(self):  # type: ignore
        return np.linspace(0, self.n_points() / self.samplerate(), self.n_points())


class GeneratedBinArray(Parameter):
    """A parameter that generates a bin array from the n_bins
    parameters.
    """

    def __init__(self, n_bins, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.n_bins = n_bins

    def get_raw(self):  # type: ignore
        return np.linspace(0, self.n_bins(), self.n_bins())


class GeneratedTurboTimeArray(Parameter):
    """A parameter that generates a time array from sample rate and number of points
    parameters.
    """

    def __init__(self, samplerate, n_points, ny_bins, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.samplerate = samplerate
        self.n_points = n_points

    def get_raw(self):  # type: ignore
        return np.linspace(0, self.n_points() / self.samplerate(), self.n_points())


class GeneratedTurboBinArray(Parameter):
    """A parameter that generates a bin array from the nx_bins and ny_bins
    parameters.
    """

    def __init__(self, nx_bins, ny_bins, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.nx_bins = nx_bins
        self.ny_bins = ny_bins

    def get_raw(self):  # type: ignore
        return np.linspace(0, self.nx_bins(), self.nx_bins())[
            :, np.newaxis
        ] + np.linspace(0, self.ny_bins(), self.ny_bins())


class collect_traces(ParameterWithSetpoints):
    def __init__(self, samplerate, n_points, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.samplerate = samplerate
        self.n_points = n_points

    def get_raw(self):  # type: ignore
        return np.linspace(0, self.n_points() / self.samplerate(), self.n_points())


class collect_BinArray(ParameterWithSetpoints):
    """A parameter that generates a bin array from the n_bins
    parameters.
    """

    def __init__(self, n_bins, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.n_bins = n_bins

    def get_raw(self):  # type: ignore
        return np.linspace(0, self.n_bins(), self.n_bins())


class collect_turbo_traces(ParameterWithSetpoints):
    def __init__(self, samplerate, n_points, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.samplerate = samplerate
        self.n_points = n_points

    def get_raw(self):  # type: ignore
        return np.linspace(
            start=0, stop=self.n_points() / self.samplerate(), num=self.n_points()
        )


class collect_Bin_turboArray(ParameterWithSetpoints):
    """A parameter that generates a turbo bin array from the nx_bin, ny_bins
    parameters.
    """

    def __init__(self, nx_bins, ny_bins, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.nx_bins = nx_bins
        self.ny_bins = ny_bins

    def get_raw(self):  # type: ignore
        return np.linspace(0, self.nx_bins(), self.nx_bins())[
            :, np.newaxis
        ] + np.linspace(0, self.ny_bins(), self.ny_bins())


class NiDAQ_ai_channel(InstrumentChannel):
    """A single analog input (ai) channel of the NiDAQ6216"""

    current_channel_name = ""

    def __init__(self, parent: "NiDAQ6216", name: str, channum: int):
        """Args:
        parent: The instrument to which the channel belongs.
        name: The name of the channel
        channum: The number of the channel (0-7)
        """
        super().__init__(parent, name)

        self.add_parameter(
            name="enable",
            label=f"ai{channum} enable",
            initial_value=False,
            vals=vals.Bool(),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            name="botVrail",
            label=f"ai{channum} bottom V rail",
            unit="V",
            initial_value=-10,
            vals=vals.Numbers(-10, 10),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            name="upVrail",
            label=f"ai{channum} upper V rail",
            unit="V",
            initial_value=10,
            vals=vals.Numbers(-10, 10),
            get_cmd=None,
            set_cmd=None,
        )
        """
        Gain and offset takes the form of gain*(input + offset)
        """

        self.add_parameter(
            name="gain",
            label=f"ai{channum} gain",
            unit="A/V",
            initial_value=-1e-9,
            vals=vals.Numbers(-1e-3, 1e-3),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            name="offset",
            label=f"ai{channum} offset",
            unit="V",
            initial_value=0,
            vals=vals.Numbers(-10, 10),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            name="voltage_traces",
            label=f"ai{channum} voltage traces",
            unit="A",
            setpoints=(self._parent.parameters["time_axis"],),
            parameter_class=collect_traces,
            samplerate=self._parent.parameters["sample_rate"],
            n_points=self._parent.parameters["n_points"],
            vals=vals.Arrays(shape=(self._parent.parameters["n_points"].get_latest,)),
        )
        # aver_value is used only for plot1D and plot2D
        self.add_parameter(
            name="aver_value",
            label=f"ai{channum} averaged trace value",
            unit="A",
            vals=vals.Numbers(),
        )

        self.add_parameter(
            name="aver_voltage",
            label=f"ai{channum} averaged trace",
            unit="A",
            setpoints=(self._parent.parameters["bin_axis"],),
            parameter_class=collect_BinArray,
            n_bins=self._parent.parameters["n_bins"],
            vals=vals.Arrays(shape=(self._parent.parameters["n_bins"].get_latest,)),
        )

        self.add_parameter(
            name="turbo_array",
            label=f"ai{channum} turbo array",
            unit="A",
            setpoints=(self._parent.parameters["turbo_time"],),
            parameter_class=collect_turbo_traces,
            samplerate=self._parent.parameters["sample_rate"],
            n_points=self._parent.parameters["n_points"],
            vals=vals.Arrays(shape=(self._parent.parameters["n_points"].get_latest,)),
        )

        self.add_parameter(
            name="aver_turbo_array",
            label=f"ai{channum} averaged turbo x array",
            unit="A",
            setpoints=(
                self._parent.parameters["turbo_x_bin_array"],
                self._parent.parameters["turbo_y_bin_array"],
            ),
            parameter_class=collect_Bin_turboArray,
            nx_bins=self._parent.parameters["nx_bins"],
            ny_bins=self._parent.parameters["ny_bins"],
            vals=vals.Arrays(
                shape=(
                    self._parent.parameters["nx_bins"].get_latest,
                    self._parent.parameters["ny_bins"].get_latest,
                )
            ),
        )


class NiDAQ_pfi_channel(InstrumentChannel):
    """A single digital pfi channel of the NiDAQ6216

    Here these are only coded to support input triggering

    TODO: allow for output triggering
    """

    current_channel_name = ""

    def __init__(self, parent: "NiDAQ6216", name: str, channum: int):
        """Args:
        parent: The instrument to which the channel belongs.
        name: The name of the channel
        channum: The number of the channel (0-7)
        """
        super().__init__(parent, name)

        self.add_parameter(
            name="enable",
            label=f"pfi channel {channum} enable",
            initial_value=False,
            vals=vals.Bool(),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            name="trigger_edge",
            label=f"pfi channel {channum} edge trigger",
            initial_value=Edge.RISING,
            vals=vals.Enum(Edge.FALLING, Edge.RISING),
            get_cmd=None,
            set_cmd=None,
        )


class NiDAQ6216(Instrument):
    """Channelised driver for the NiDAQ6216

    Exposes ai and pfi channels

    TODO: make compatible with ao channels
    """

    _index_df = pd.DataFrame()

    def __init__(self, name: str, NiMAXname: str, **kwargs):
        """Instantiated the instrument.

        Args:
            name: The instrument name used in qcodes
            NiMAXname: the name under the settings column of niMAX with the NI USB-6216 selected

        Returns:
            NiDAQ6216 object
        """
        super().__init__(name, **kwargs)
        self.NiMAXname = NiMAXname
        self.add_parameter(
            "sample_rate",
            unit="samples/sec",
            label="sample rate",
            initial_value=1000,
            vals=vals.Numbers(1, 100e3),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            "n_points",
            unit="",
            initial_value=11,
            vals=vals.Numbers(1, 1e6),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            name="n_bins",
            label="number of averaged steps",
            initial_value=1,
            vals=vals.Numbers(1, 1e3),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            name="nx_bins",
            label="number of averaged x steps",
            initial_value=1,
            vals=vals.Numbers(1, 1e3),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            name="ny_bins",
            label="number of averaged y steps",
            initial_value=1,
            vals=vals.Numbers(1, 1e3),
            get_cmd=None,
            set_cmd=None,
        )

        self.add_parameter(
            name="time_axis",
            label="time array",
            unit="sec",
            parameter_class=GeneratedTimeArray,
            samplerate=self.sample_rate,
            n_points=self.n_points,
            vals=vals.Arrays(shape=(self.n_points.get_latest,)),
        )

        self.add_parameter(
            name="turbo_time",
            label="turbo time array",
            unit="sec",
            parameter_class=GeneratedTurboTimeArray,
            samplerate=self.sample_rate,
            n_points=self.n_points,
            ny_bins=self.ny_bins,
            vals=vals.Arrays(shape=(self.n_points.get_latest,)),
        )

        self.add_parameter(
            name="bin_axis",
            label="binned axis array",
            unit="",
            parameter_class=GeneratedBinArray,
            n_bins=self.n_bins,
            vals=vals.Arrays(shape=(self.n_bins.get_latest,)),
        )

        self.add_parameter(
            name="turbo_x_bin_array",
            label="binned x axis array",
            unit="V",
            parameter_class=GeneratedTurboBinArray,
            nx_bins=self.nx_bins,
            ny_bins=self.ny_bins,
            vals=vals.Arrays(shape=(self.nx_bins.get_latest, self.ny_bins.get_latest)),
        )

        self.add_parameter(
            name="turbo_y_bin_array",
            label="binned y axis array",
            unit="V",
            parameter_class=GeneratedTurboBinArray,
            nx_bins=self.nx_bins,
            ny_bins=self.ny_bins,
            vals=vals.Arrays(shape=(self.nx_bins.get_latest, self.ny_bins.get_latest)),
        )

        # Initialize basic information and internal book keeping
        ai_channels = ChannelList(
            self, "AI_Channels", NiDAQ_ai_channel, snapshotable=True
        )

        for i in range(0, 8):
            ai_channel = NiDAQ_ai_channel(self, f"ai{i}", channum=i)
            ai_channels.append(ai_channel)
            self.add_submodule(f"ai{i}", ai_channel)
        self.add_submodule("ai_channels", ai_channels.to_channel_tuple())

        pfi_channels = ChannelList(
            self, "PFI_Channels", NiDAQ_pfi_channel, snapshotable=True
        )

        for i in range(0, 8):
            pfi_channel = NiDAQ_pfi_channel(self, f"pfi{i}", i)
            pfi_channels.append(pfi_channel)
            self.add_submodule(f"pfi{i}", pfi_channel)
        self.add_submodule("pfi_channels", pfi_channels.to_channel_tuple())

    def write(self, cmd: str) -> None:
        """QDac always returns something even from set commands, even when
        verbose mode is off, so we'll override write to take this out
        if you want to use this response, we put it in self._write_response
        (but only for the very last write call)

        In this method we expect to read one termination char per command. As
        commands are concatenated by `;` we count the number of concatenated
        commands as count(';') + 1 e.g. 'wav 1 1 1 0;fun 2 1 100 1 1' is two
        commands. Note that only the response of the last command will be
        available in `_write_response`
        """
        LOG.debug(f"Writing to instrument {self.name}: {cmd}")
        self.visa_handle.write(cmd)
        self._write_response = self.visa_handle.read()
        if self._write_response.startswith("Error: "):
            LOG.warning(self._write_response)

    def read(self) -> str:
        return self.visa_handle.read()

    def connect_message(  # type: ignore
        self,
        idn_param: str = "*IDN?",
    ) -> None:
        """Override of the standard Instrument class connect_message.
        Usually, the response to `*IDN?` is printed. Here, the
        software version is printed.
        """
        # self.visa_handle.write(idn_param)
        LOG.info("Connected to NiDAQ")

    def return_ai_channels(self):
        """Creates and returns a list of all enabled analog inputs"""
        return [ch for ch in self.ai_channels if ch.enable.get()]

    def return_pfi_channels(self):
        """Creates and returns a list of all enabled pfi channels"""
        return [ch for ch in self.pfi_channels if ch.enable.get()]


class NDictionary:
    """Class for storage and organization of multiple NIDAQ6216 objects for an entire experiment"""

    _index_df = pd.DataFrame()
    _nidaqs: dict[str, NiDAQ6216]

    def __init__(self, nidaqs: dict[str, NiDAQ6216], Xcel_dict):
        self._index_df = Xcel_dict
        self._nidaqs = nidaqs

    def _return_ai_channel_object(
        self,
        pad: str,
    ):
        """Returns NIDAQ ai channel object based on pad input from wiremap"""
        row = self._index_df[self._index_df["Pad"] == pad]
        nidaq = str(list(row["NIDaq"])[0])
        channel = int(str(list(row["NIDaq Ch"])[0])[2:])
        return self._nidaqs[nidaq].ai_channels[channel]

    def _return_pfi_channel_object(
        self,
        pad: str,
    ):
        """Returns NIDAQ pfi channel object based on pad input from wiremap"""
        row = self._index_df[self._index_df["Pad"] == pad]
        nidaq = str(list(row["NIDaq"])[0])
        channel = int(str(list(row["NIDaq Ch"])[0])[3:])
        return self._nidaqs[nidaq].pfi_channels[channel]

    def _return_pfi_channel_object_from_NIDaq_Ch(
        self,
        ch: str,
    ):
        """Returns NIDAQ pfi channel object based on pad input from wiremap"""
        row = self._index_df[self._index_df["NIDaq Ch"] == ch]
        nidaq = str(list(row["NIDaq"])[0])
        channel = int(str(list(row["NIDaq Ch"])[0])[3:])
        return self._nidaqs[nidaq].pfi_channels[channel]

    def _return_nidaq_name_from_pfi(
        self,
        pad: str,
    ):
        """Returns NIDAQ pfi channel object based on pad input from wiremap"""
        row = self._index_df[self._index_df["Pad"] == pad]
        nidaq = str(list(row["NIDaq"])[0])
        return nidaq

    def _return_nidaq_object_from_name(
        self,
        name: str,
    ):
        """Returns the qdac object based off an aribtrary name"""
        return self._nidaqs[name]

    def trace_averaging(self, data, step_length, risetime):
        """Implements the averaging of time traces from NIDAQ6216

        INPUTS
        data : output dictionary holding data formatted by Waveform1D
        step_length : the step length in msec that the averaging is going over
        risetime : the estimated RC risetime that will be ignored during averaging
        """
        package_data = {}
        for nidaq in list(self._nidaqs.keys()):
            nsteps = self._return_nidaq_object_from_name(nidaq).n_bins.get()
            ave_samples = int(
                step_length
                * self._return_nidaq_object_from_name(nidaq).sample_rate.get()
                / 1000
            )
            rising_percentage_of_sweep = risetime / step_length  # max value should be 1
            ignore_samples = int(rising_percentage_of_sweep * ave_samples)
            for currents in data.keys():
                aver_array = np.zeros(nsteps)
                if (currents != "time") or (currents != "voltage"):
                    for i in range(0, nsteps):
                        aver_array[i] = np.average(
                            data[currents][
                                i * ave_samples + ignore_samples : (i + 1) * ave_samples
                            ]
                        )
                    package_data[currents] = [data[currents], aver_array]
            package_data["time"] = data["time"]
            # package_data["voltage"] = data["voltage"]
        return package_data

    def trace_turbo_averaging(
        self, data, step_length, risetime, turboVoltagetimeLength, verbose=False
    ):
        """Implements the averaging of turbo time traces from NIDAQ6216

        INPUTS
        data : output dictionary holding data formatted by Waveform1D
        step_length : array holdilng the step length in msec that the averaging is going over
        risetime : the estimated RC risetime that will be ignored during averaging
        turboVoltagetimeLength : the length of the turbo time array
        """
        package_data = {}
        for nidaq in list(self._nidaqs.keys()):
            nxsteps = self._return_nidaq_object_from_name(nidaq).nx_bins.get()
            nysteps = self._return_nidaq_object_from_name(nidaq).ny_bins.get()
            ave_samples = int(
                step_length
                * self._return_nidaq_object_from_name(nidaq).sample_rate.get()
                / 1000
            )
            rising_percentage_of_sweep = risetime / step_length  # max value should be 1
            ignore_samples = int(rising_percentage_of_sweep * ave_samples)
            skip_ramp_down = int(
                turboVoltagetimeLength
                * self._return_nidaq_object_from_name(nidaq).sample_rate.get()
                / 1000
            ) - (nxsteps * ave_samples)
            if verbose:
                print(f"The turbo Voltage time length is {turboVoltagetimeLength}")
            if verbose:
                print(
                    f"The number of steps skipped during ramp down is {skip_ramp_down}"
                )

            len_of_xtime = int(len(data["time"]) / nysteps)
            for currents in data.keys():
                if verbose:
                    print(f"This is current {currents}")
                reshaped_data = []
                data_stripped_ramp_down = []
                aver_array = []
                aver_array = np.zeros((nysteps, nxsteps))
                if (currents != "time") and (currents != "voltage"):
                    reshaped_data = np.reshape(
                        a=data[currents], newshape=(nysteps, len_of_xtime)
                    )
                    # print(reshaped_data.shape)
                    data_stripped_ramp_down = reshaped_data[:, : ave_samples * nxsteps]
                    # print(data_stripped_ramp_down)
                    for i in range(0, nxsteps):
                        aver_array[:, i] = np.average(
                            data_stripped_ramp_down[
                                :, i * ave_samples : (i + 1) * ave_samples
                            ],
                            axis=1,
                        )
                    if verbose:
                        print(f" shape of aver array {aver_array.shape}")
                package_data[currents] = [data[currents], aver_array]
            package_data["time"] = data["time"]
        return package_data

    def yaxis_labels(self):
        """Fixes y axis (ai input channels of NiDAQ) to current_channel labels from index_df"""
        for current in list(
            self._index_df[self._index_df["Description"] == "Iamp"]["Pad"]
        ):
            self._return_ai_channel_object(current).voltage_traces.label = current
            self._return_ai_channel_object(current).aver_voltage.label = current
            self._return_ai_channel_object(current).turbo_array.label = current
            self._return_ai_channel_object(current).aver_turbo_array.label = current
            self._return_ai_channel_object(current).aver_value.label = current

    def xaxis_labels(
        self,
        channel1pads,
        channel2pads=None,
        name1=None,
        name2=None,
        unit1="V",
        unit2="V",
    ):
        """TODO define this function compatible with turbo

        Fixes x axis (ai input channels of NiDAQ) to supplied name, or first pad in list of pads

        INPUTS
        channel1pads : list of pads associated with channel1
        channel2pads : list of pads associated with channel2
        name1 : optional name for x axis associated with the group of pads
        name2 : optional name for y axis associated with the group of pads
        unit1 : the units for the x axis
        unti2 : the units for the y axis
        """
        if channel2pads is None:
            if name1 is None:
                for NiDaq in list(self._nidaqs.keys()):
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).bin_axis.label = channel1pads[0]
                    self._return_nidaq_object_from_name(NiDaq).bin_axis.unit = "V"

            else:
                for NiDaq in list(self._nidaqs.keys()):
                    self._return_nidaq_object_from_name(NiDaq).bin_axis.label = name1
                    self._return_nidaq_object_from_name(NiDaq).bin_axis.unit = unit1
        else:  # channel2pads actually holds something
            # We need to do this twice, once for channel 1 and another for channel 2
            if name1 is None:
                for NiDaq in list(self._nidaqs.keys()):
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).turbo_x_bin_array.label = channel1pads[0]
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).turbo_x_bin_array.unit = "V"
                    # Here sweep2D is possible so
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).bin_axis.label = channel1pads[0]
                    self._return_nidaq_object_from_name(NiDaq).bin_axis.unit = "V"

            else:
                for NiDaq in list(self._nidaqs.keys()):
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).turbo_x_bin_array.label = name1
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).turbo_x_bin_array.unit = unit1
                    # Here sweep2D is possible so
                    self._return_nidaq_object_from_name(NiDaq).bin_axis.label = name1
                    self._return_nidaq_object_from_name(NiDaq).bin_axis.unit = unit1

            if name2 is None:
                for NiDaq in list(self._nidaqs.keys()):
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).turbo_y_bin_array.label = channel2pads[0]
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).turbo_y_bin_array.unit = "V"

            else:
                for NiDaq in list(self._nidaqs.keys()):
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).turbo_y_bin_array.label = name2
                    self._return_nidaq_object_from_name(
                        NiDaq
                    ).turbo_y_bin_array.unit = unit2
